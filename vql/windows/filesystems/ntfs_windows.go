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

	"github.com/Velocidex/ordereddict"
	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/third_party/cache"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/windows/wmi"
	"www.velocidex.com/golang/vfilter"
)

const (
	NTFSFileSystemTag = "_NTFS"
)

type AccessorContext struct {
	reader   *ntfs.PagedReader
	fd       *os.File
	ntfs_ctx *ntfs.NTFSContext

	path_listing *cache.LRUCache
}

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
	result := ordereddict.NewDict().
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

func (self *NTFSFileInfo) Mtime() utils.TimeVal {
	nsec := self.info.Mtime.UnixNano()
	return utils.TimeVal{
		Sec:  nsec / 1000000000,
		Nsec: nsec,
	}
}

func (self *NTFSFileInfo) Ctime() utils.TimeVal {
	nsec := self.info.Ctime.UnixNano()
	return utils.TimeVal{
		Sec:  nsec / 1000000000,
		Nsec: nsec,
	}
}

func (self *NTFSFileInfo) Atime() utils.TimeVal {
	nsec := self.info.Atime.UnixNano()
	return utils.TimeVal{
		Sec:  nsec / 1000000000,
		Nsec: nsec,
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
		Mtime    utils.TimeVal
		Ctime    utils.TimeVal
		Atime    utils.TimeVal
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

type NTFSFileSystemAccessor struct {
	// Cache raw devices for a given time. Note that the cache is
	// only alive for the duration of a single VQL query
	// (including its subqueries). The query will close the cache
	// after it terminates. The cache helps in the case where
	// subqueries need to open the same device. For long running
	// queries, the cache will refresh every 10 minutes to get a
	// fresh view of the data.
	mu        sync.Mutex
	fd_cache  map[string]*AccessorContext // Protected by mutex
	timestamp time.Time                   // Protected by mutex
}

func (self NTFSFileSystemAccessor) New(scope *vfilter.Scope) (glob.FileSystemAccessor, error) {
	result_any := vql_subsystem.CacheGet(scope, NTFSFileSystemTag)
	if result_any == nil {
		// Create a new cache in the scope.
		result := &NTFSFileSystemAccessor{
			fd_cache: make(map[string]*AccessorContext),
		}

		vql_subsystem.CacheSet(scope, NTFSFileSystemTag, result)

		// When scope is destroyed, we close all the filehandles.
		scope.AddDestructor(func() {
			result.mu.Lock()
			defer result.mu.Unlock()

			for _, v := range result.fd_cache {
				v.fd.Close()
			}

			result.fd_cache = make(map[string]*AccessorContext)
			vql_subsystem.CacheSet(scope, NTFSFileSystemTag, result)
		})
		return result, nil
	}

	return result_any.(glob.FileSystemAccessor), nil
}

func (self *NTFSFileSystemAccessor) getRootMFTEntry(ntfs_ctx *ntfs.NTFSContext) (
	*ntfs.MFT_ENTRY, error) {
	return ntfs_ctx.GetMFT(5)
}

func (self *NTFSFileSystemAccessor) getNTFSContext(device string) (
	*AccessorContext, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// We cache the paged reader as well as the original file
	// handle so we can safely close it when the query is done.
	cached_ctx, pres := self.fd_cache[device]
	if !pres || time.Now().After(self.timestamp.Add(10*time.Minute)) {
		// Try to open the device and list its path.
		raw_fd, err := os.OpenFile(device, os.O_RDONLY, os.FileMode(0666))
		if err != nil {
			return nil, err
		}

		reader, _ := ntfs.NewPagedReader(raw_fd, 8*1024, 1000)
		if err != nil {
			return nil, err
		}

		ntfs_ctx, err := ntfs.GetNTFSContext(reader, 0)
		if err != nil {
			return nil, err
		}
		if cached_ctx != nil {
			cached_ctx.fd.Close()
		}

		cached_ctx = &AccessorContext{
			reader:       reader,
			fd:           raw_fd,
			ntfs_ctx:     ntfs_ctx,
			path_listing: cache.NewLRUCache(200),
		}
		self.fd_cache[device] = cached_ctx
		self.timestamp = time.Now()
	}
	return cached_ctx, nil
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

	accessor_ctx, err := self.getNTFSContext(device)
	if err != nil {
		return nil, err
	}

	ntfs_ctx := accessor_ctx.ntfs_ctx
	root, err := ntfs_ctx.GetMFT(5)
	if err != nil {
		return nil, err
	}

	// Open the device path from the root.
	dir, err := Open(root, accessor_ctx, device, subpath)
	if err != nil {
		return nil, err
	}

	// Only process each mft id once.
	seen := []int64{}
	in_seen := func(id int64) bool {
		for _, i := range seen {
			if i == id {
				return true
			}
		}
		return false
	}

	// List the directory.
	for _, node := range dir.Dir(ntfs_ctx) {
		node_mft_id := int64(node.MftReference())
		if in_seen(node_mft_id) {
			continue
		}

		seen = append(seen, node_mft_id)

		node_mft, err := ntfs_ctx.GetMFT(node_mft_id)
		if err != nil {
			continue
		}
		// Emit a result for each filename
		for _, info := range ntfs.Stat(ntfs_ctx, node_mft) {
			full_path := device + subpath + "\\" + info.Name
			result = append(result, &NTFSFileInfo{
				info:       info,
				_full_path: full_path,
			})
		}
	}
	return result, nil
}

type readAdapter struct {
	sync.Mutex

	info   glob.FileInfo
	reader io.ReaderAt
	pos    int64
}

func (self *readAdapter) Read(buf []byte) (res int, err error) {
	self.Lock()
	defer self.Unlock()

	defer func() {
		r := recover()
		if r != nil {
			fmt.Printf("PANIC %v\n", r)
			err, _ = r.(error)
		}
	}()

	res, err = self.reader.ReadAt(buf, self.pos)
	self.pos += int64(res)

	// If ReadAt is unable to read anything it means an EOF.
	if res == 0 {
		return res, io.EOF
	}

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

	accessor_ctx, err := self.getNTFSContext(device)
	if err != nil {
		return nil, err
	}

	ntfs_ctx := accessor_ctx.ntfs_ctx
	root, err := self.getRootMFTEntry(ntfs_ctx)
	if err != nil {
		return nil, err
	}

	data, err := ntfs.GetDataForPath(ntfs_ctx, subpath)
	if err != nil {
		return nil, err
	}

	dirname := filepath.Dir(subpath)
	dir, err := Open(root, accessor_ctx, device, dirname)
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

	accessor_ctx, err := self.getNTFSContext(device)
	if err != nil {
		return nil, err
	}

	ntfs_ctx := accessor_ctx.ntfs_ctx
	root, err := self.getRootMFTEntry(ntfs_ctx)
	if err != nil {
		return nil, err
	}

	dirname := filepath.Dir(subpath)
	dir, err := Open(root, accessor_ctx, device, dirname)
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
	return paths.GetDeviceAndSubpath(path)
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

// Open the MFT entry specified by a path name. Walks all directory
// indexes in the path to find the right MFT entry.
func Open(self *ntfs.MFT_ENTRY, accessor_ctx *AccessorContext, device, filename string) (
	*ntfs.MFT_ENTRY, error) {

	ntfs_ctx := accessor_ctx.ntfs_ctx

	components := utils.SplitComponents(filename)

	// Path is the relative path from the root of the device we want to list
	// component: The name of the file we want (case insensitive)
	// dir: The MFT entry to search.
	get_path_in_dir := func(path string, component string, dir *ntfs.MFT_ENTRY) (
		*ntfs.MFT_ENTRY, error) {

		key := device + path
		item, pres := GetDirLRU(accessor_ctx, key, component)
		if pres {
			return ntfs_ctx.GetMFT(item.mft_id)
		}

		lru_map := make(map[string]*cacheMFT)

		// Populate the directory cache with all the mft ids.
		lower_component := strings.ToLower(component)
		for _, idx_record := range dir.Dir(ntfs_ctx) {
			file := idx_record.File()
			name_type := file.NameType().Name
			if name_type == "DOS" {
				continue
			}
			item_name := file.Name()
			mft_id := int64(idx_record.MftReference())

			lru_map[strings.ToLower(item_name)] = &cacheMFT{
				mft_id:    mft_id,
				component: item_name,
				name_type: name_type,
			}
		}
		SetLRUMap(accessor_ctx, key, lru_map)

		for _, v := range lru_map {
			if strings.ToLower(v.component) == lower_component {
				return ntfs_ctx.GetMFT(v.mft_id)
			}
		}

		return nil, errors.New("Not found")
	}

	// NOTE: This refreshes each parent directory in the LRU.
	directory := self
	path := ""
	for _, component := range components {
		if component == "" {
			continue
		}
		next, err := get_path_in_dir(
			path, component, directory)
		if err != nil {
			return nil, err
		}
		directory = next
		path = path + "\\" + component
	}

	return directory, nil
}

type cacheMFT struct {
	component string
	mft_id    int64
	name_type string
}

type cacheElement struct {
	children map[string]*cacheMFT
}

func (self cacheElement) Size() int {
	return 1
}

func SetLRUMap(accessor_ctx *AccessorContext, path string, lru_map map[string]*cacheMFT) {
	accessor_ctx.path_listing.Set(path, cacheElement{children: lru_map})
}

func GetDirLRU(accessor_ctx *AccessorContext, path string, component string) (
	*cacheMFT, bool) {
	value, pres := accessor_ctx.path_listing.Get(path)
	if pres {
		item, pres := value.(cacheElement).children[strings.ToLower(component)]
		return item, pres
	}

	return nil, false
}

func init() {
	glob.Register("ntfs", &NTFSFileSystemAccessor{})
}
