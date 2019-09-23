/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
// A Raw NTFS accessor for disks.

// The NTFS accessor provides access to volumes, and Volume Shadow
// Copies through the VSS devices.

package filesystems

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/vql/parsers"
	"www.velocidex.com/golang/velociraptor/vql/windows/wmi"
	"www.velocidex.com/golang/vfilter"
)

var (
	// Cache raw devices for a given time. Note that the cache is
	// only alive for the duration of a single VQL query
	// (including its subqueries). The query will close the cache
	// after it terminates. The cache helps in the case where
	// subqueries need to open the same device. For long running
	// queries, the cache will refresh every 10 minutes to get a
	// fresh view of the data.
	mu        sync.Mutex
	fd_cache  map[string]*PagedReader // Protected by mutex
	timestamp time.Time               // Protected by mutex
)

type NTFSFileInfo struct {
	info       *ntfs.FileInfo
	_full_path string
}

func (self *NTFSFileInfo) IsDir() bool {
	return self.info.IsDir
}

func (self *NTFSFileInfo) Size() int64 {
	return self.info.Size
}

func (self *NTFSFileInfo) Data() interface{} {
	result := vfilter.NewDict().
		Set("mft", self.info.MFTId).
		Set("name_type", self.info.NameType)
	if self.info.ExtraNames != nil {
		result.Set("extra_names", self.info.ExtraNames)
	}

	return result
}

func (self *NTFSFileInfo) Name() string {
	return self.info.Name
}

func (self *NTFSFileInfo) Sys() interface{} {
	return self.Data()
}

func (self *NTFSFileInfo) Mode() os.FileMode {
	var result os.FileMode = 0755
	if self.IsDir() {
		result |= os.ModeDir
	}
	return result
}

func (self *NTFSFileInfo) ModTime() time.Time {
	return self.info.Mtime
}

func (self *NTFSFileInfo) FullPath() string {
	return self._full_path
}

func (self *NTFSFileInfo) Mtime() glob.TimeVal {
	return glob.TimeVal{
		Sec: self.info.Mtime.Unix(),
	}
}

func (self *NTFSFileInfo) Ctime() glob.TimeVal {
	return glob.TimeVal{
		Sec: self.info.Ctime.Unix(),
	}
}

func (self *NTFSFileInfo) Atime() glob.TimeVal {
	return glob.TimeVal{
		Sec: self.info.Atime.Unix(),
	}
}

// Not supported
func (self *NTFSFileInfo) IsLink() bool {
	return false
}

func (self *NTFSFileInfo) GetLink() (string, error) {
	return "", errors.New("Not implemented")
}

func (self *NTFSFileInfo) MarshalJSON() ([]byte, error) {
	result, err := json.Marshal(&struct {
		FullPath string
		Size     int64
		Mode     os.FileMode
		ModeStr  string
		ModTime  time.Time
		Sys      interface{}
		Mtime    glob.TimeVal
		Ctime    glob.TimeVal
		Atime    glob.TimeVal
	}{
		FullPath: self.FullPath(),
		Size:     self.Size(),
		Mode:     self.Mode(),
		ModeStr:  self.Mode().String(),
		ModTime:  self.ModTime(),
		Sys:      self.Sys(),
		Mtime:    self.Mtime(),
		Ctime:    self.Ctime(),
		Atime:    self.Atime(),
	})

	return result, err
}

type PagedReader struct {
	*ntfs.PagedReader

	fd *os.File
}

type NTFSFileSystemAccessor struct{}

func (self NTFSFileSystemAccessor) New(ctx context.Context) glob.FileSystemAccessor {
	result := &NTFSFileSystemAccessor{}

	// When the context is done, close all the files. The files
	// must remain open until the entire VQL query is done.
	go func() {
		select {
		case <-ctx.Done():
			mu.Lock()
			defer mu.Unlock()

			for k, v := range fd_cache {
				v.fd.Close()
				delete(fd_cache, k)
			}
		}
	}()

	return result
}

func (self *NTFSFileSystemAccessor) getRootMFTEntry(ntfs_ctx *ntfs.NTFSContext) (
	*ntfs.MFT_ENTRY, error) {
	return ntfs_ctx.GetMFT(5)
}

func (self *NTFSFileSystemAccessor) getNTFSContext(device string) (
	*ntfs.NTFSContext, error) {
	mu.Lock()
	defer mu.Unlock()

	// We cache the paged reader as well as the original file
	// handle so we can safely close it when the query is done.
	fd, pres := fd_cache[device]
	if !pres || time.Now().After(timestamp.Add(10*time.Minute)) {
		// Try to open the device and list its path.
		raw_fd, err := os.OpenFile(device, os.O_RDONLY, os.FileMode(0666))
		if err != nil {
			return nil, err
		}

		reader, _ := ntfs.NewPagedReader(raw_fd, 1024, 10000)
		fd = &PagedReader{
			PagedReader: reader,
			fd:          raw_fd,
		}
		if err != nil {
			return nil, err
		}

		fd_cache[device] = fd
		timestamp = time.Now()
	}

	return ntfs.GetNTFSContext(fd, 0)
}

func discoverVSS() ([]glob.FileInfo, error) {
	result := []glob.FileInfo{}

	shadow_volumes, err := wmi.Query(
		"SELECT DeviceObject, VolumeName, InstallDate, "+
			"OriginatingMachine from Win32_ShadowCopy",
		"ROOT\\CIMV2")
	if err == nil {
		for _, row := range shadow_volumes {
			k, pres := row.Get("DeviceObject")
			if pres {
				device_name, ok := k.(string)
				if ok {
					virtual_directory := glob.NewVirtualDirectoryPath(
						device_name, row)
					result = append(result, virtual_directory)
				}
			}
		}
	}

	return result, nil
}

func discoverLogicalDisks() ([]glob.FileInfo, error) {
	result := []glob.FileInfo{}

	shadow_volumes, err := wmi.Query(
		"SELECT DeviceID, Description, VolumeName, FreeSpace, "+
			"Size, SystemName, VolumeSerialNumber "+
			"from Win32_LogicalDisk WHERE FileSystem = 'NTFS'",
		"ROOT\\CIMV2")
	if err == nil {
		for _, row := range shadow_volumes {
			k, pres := row.Get("DeviceID")
			if pres {
				device_name, ok := k.(string)
				if ok {
					virtual_directory := glob.NewVirtualDirectoryPath(
						"\\\\.\\"+device_name, row)
					result = append(result, virtual_directory)
				}
			}
		}
	}

	return result, nil
}

func (self *NTFSFileSystemAccessor) ReadDir(path string) (res []glob.FileInfo, err error) {
	defer func() {
		r := recover()
		if r != nil {
			fmt.Printf("PANIC %v\n", r)
			err, _ = r.(error)
		}
	}()

	result := []glob.FileInfo{}

	// The path must start with a valid device, otherwise we list
	// the devices.
	device, subpath, err := self.GetRoot(path)
	if err != nil {
		vss, err := discoverVSS()
		if err == nil {
			result = append(result, vss...)
		}

		logical, err := discoverLogicalDisks()
		if err == nil {
			result = append(result, logical...)
		}

		return result, nil
	}

	ntfs_ctx, err := self.getNTFSContext(device)
	if err != nil {
		return nil, err
	}

	root, err := ntfs_ctx.GetMFT(5)
	if err != nil {
		return nil, err
	}

	// Open the device path from the root.
	dir, err := root.Open(ntfs_ctx, subpath)
	if err != nil {
		return nil, err
	}

	// List the directory.
	for _, info := range ntfs.ListDir(ntfs_ctx, dir) {
		if info.Name == "" || info.Name == "." {
			continue
		}
		result = append(result, &NTFSFileInfo{
			info:       info,
			_full_path: device + subpath + "\\" + info.Name,
		})
	}
	return result, nil
}

type readAdapter struct {
	sync.Mutex

	info   *NTFSFileInfo
	reader io.ReaderAt
	pos    int64
}

func (self *readAdapter) Read(buf []byte) (int, error) {
	self.Lock()
	defer self.Unlock()
	res, err := self.reader.ReadAt(buf, self.pos)
	self.pos += int64(res)

	return res, err
}

func (self *readAdapter) ReadAt(buf []byte, offset int64) (int, error) {
	self.Lock()
	defer self.Unlock()
	self.pos = offset

	return self.reader.ReadAt(buf, offset)
}

func (self *readAdapter) Close() error {
	return nil
}

func (self *readAdapter) Stat() (os.FileInfo, error) {
	self.Lock()
	defer self.Unlock()

	return self.info, nil
}

func (self *readAdapter) Seek(offset int64, whence int) (int64, error) {
	self.Lock()
	defer self.Unlock()

	self.pos = offset
	return self.pos, nil
}

func (self *NTFSFileSystemAccessor) Open(path string) (res glob.ReadSeekCloser, err error) {
	defer func() {
		r := recover()
		if r != nil {
			fmt.Printf("PANIC %v\n", r)
			err, _ = r.(error)
		}
	}()

	// The path must start with a valid device, otherwise we list
	// the devices.
	device, subpath, err := self.GetRoot(path)
	if err != nil {
		return nil, errors.New("Unable to open raw device")
	}

	components := self.PathSplit(subpath)

	ntfs_ctx, err := self.getNTFSContext(device)
	if err != nil {
		return nil, err
	}

	root, err := self.getRootMFTEntry(ntfs_ctx)
	if err != nil {
		return nil, err
	}

	data, err := ntfs.GetDataForPath(ntfs_ctx, subpath)
	if err != nil {
		return nil, err
	}

	dirname := filepath.Dir(subpath)
	dir, err := root.Open(ntfs_ctx, dirname)
	if err != nil {
		return nil, err
	}

	for _, info := range ntfs.ListDir(ntfs_ctx, dir) {
		if strings.ToLower(info.Name) == strings.ToLower(
			components[len(components)-1]) {
			return &readAdapter{
				info: &NTFSFileInfo{
					info:       info,
					_full_path: device + dirname + "\\" + info.Name,
				},
				reader: data,
			}, nil
		}
	}

	return nil, errors.New("File not found")
}

func (self *NTFSFileSystemAccessor) Lstat(path string) (res glob.FileInfo, err error) {
	defer func() {
		r := recover()
		if r != nil {
			fmt.Printf("PANIC %v\n", r)
			err, _ = r.(error)
		}
	}()

	// The path must start with a valid device, otherwise we list
	// the devices.
	device, subpath, err := self.GetRoot(path)
	if err != nil {
		return nil, errors.New("Unable to open raw device")
	}

	components := self.PathSplit(subpath)

	ntfs_ctx, err := self.getNTFSContext(device)
	if err != nil {
		return nil, err
	}

	root, err := self.getRootMFTEntry(ntfs_ctx)
	if err != nil {
		return nil, err
	}

	dirname := filepath.Dir(subpath)
	dir, err := root.Open(ntfs_ctx, dirname)
	if err != nil {
		return nil, err
	}
	for _, info := range ntfs.ListDir(ntfs_ctx, dir) {
		if strings.ToLower(info.Name) == strings.ToLower(
			components[len(components)-1]) {
			return &NTFSFileInfo{
				info:       info,
				_full_path: device + dirname + "\\" + info.Name,
			}, nil
		}
	}

	return nil, errors.New("File not found")
}

func (self *NTFSFileSystemAccessor) GetRoot(path string) (
	device string, subpath string, err error) {
	return parsers.GetDeviceAndSubpath(path)
}

// We accept both / and \ as a path separator
var NTFSFileSystemAccessor_re = regexp.MustCompile("[\\\\/]")

func (self *NTFSFileSystemAccessor) PathSplit(path string) []string {
	return NTFSFileSystemAccessor_re.Split(path, -1)
}

func (self NTFSFileSystemAccessor) PathJoin(x, y string) string {
	return x + "\\" + strings.TrimLeft(y, "\\")
}

// We want to show the entire device as one name so we need to escape
// \\ characters so they are not interpreted as a path separator.
func escape(path string) string {
	result := strings.Replace(path, "\\", "%5c", -1)
	return strings.Replace(result, "/", "%2f", -1)
}

func unescape(path string) string {
	result := strings.Replace(path, "%5c", "\\", -1)
	return strings.Replace(result, "%2f", "/", -1)
}

func init() {
	glob.Register("ntfs", &NTFSFileSystemAccessor{})

	fd_cache = make(map[string]*PagedReader)
	timestamp = time.Now()
}
