// A Raw NTFS accessor for disks.

// The NTFS accessor provides access to volumes, and Volume Shadow
// Copies through the VSS devices.

package filesystems

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	ntfs "www.velocidex.com/golang/go-ntfs"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/vql/windows"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vtypes"
)

var (
	deviceDirectoryRegex = regexp.MustCompile(
		"(?i)(\\\\\\\\[\\?\\.]\\\\GLOBALROOT\\\\Device\\\\[^/\\\\]+)([/\\\\]?.*)")
)

type NTFSFileInfo struct {
	// Empty for files but may contain data for registry and
	// resident NTFS.
	_data      interface{}
	_mtime     glob.TimeVal
	_atime     glob.TimeVal
	_ctime     glob.TimeVal
	_name      string
	_full_path string
	_isdir     bool
	_size      int64
}

func (self *NTFSFileInfo) IsDir() bool {
	return self._isdir
}

func (self *NTFSFileInfo) Size() int64 {
	return self._size
}

func (self *NTFSFileInfo) Data() interface{} {
	return self._data
}

func (self *NTFSFileInfo) Name() string {
	return self._name
}

func (self *NTFSFileInfo) Sys() interface{} {
	return nil
}

func (self *NTFSFileInfo) Mode() os.FileMode {
	var result os.FileMode = 0755
	if self.IsDir() {
		result |= os.ModeDir
	}
	return result
}

func (self *NTFSFileInfo) ModTime() time.Time {
	return time.Time{}
}

func (self *NTFSFileInfo) FullPath() string {
	return self._full_path
}

func (self *NTFSFileInfo) Mtime() glob.TimeVal {
	return self._mtime
}

func (self *NTFSFileInfo) Ctime() glob.TimeVal {
	return self._ctime
}

func (self *NTFSFileInfo) Atime() glob.TimeVal {
	return self._atime
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

type NTFSFileSystemAccessor struct {
	profile *vtypes.Profile

	fd_cache map[string]*os.File
}

func (self NTFSFileSystemAccessor) New(ctx context.Context) glob.FileSystemAccessor {
	result := &NTFSFileSystemAccessor{
		profile:  self.profile,
		fd_cache: make(map[string]*os.File),
	}

	// When the context is done, close all the files. The files
	// must remain open until the entire VQL query is done.
	go func() {
		select {
		case <-ctx.Done():
			for _, v := range result.fd_cache {
				v.Close()
			}
		}
	}()

	return result
}

func (self *NTFSFileSystemAccessor) getMFTEntry(path string) (
	*ntfs.MFT_ENTRY, error) {
	components := strings.Split(path, "\\")
	device := unescape(components[0])

	fd, pres := self.fd_cache[device]
	if !pres {
		var err error

		// Try to open the device and list its path.
		fd, err = os.OpenFile(device, os.O_RDONLY, os.FileMode(0666))
		if err != nil {
			return nil, err
		}
		self.fd_cache[device] = fd
	}

	boot, err := ntfs.NewBootRecord(self.profile, fd, 0)
	if err != nil {
		return nil, err
	}

	mft, err := boot.MFT()
	if err != nil {
		return nil, err
	}

	// Get the root directory.
	root, err := mft.MFTEntry(5)
	if err != nil {
		return nil, err
	}

	// Open the device path from the root.
	dir, err := root.Open(strings.Join(components[1:], "/"))
	if err != nil {
		return nil, err
	}

	return dir, err
}

func discoverVSS() ([]glob.FileInfo, error) {
	result := []glob.FileInfo{}

	shadow_volumes, err := windows.Query(
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
						escape(device_name), row)
					result = append(result, virtual_directory)
				}
			}
		}
	}

	return result, nil
}

func discoverLogicalDisks() ([]glob.FileInfo, error) {
	result := []glob.FileInfo{}

	shadow_volumes, err := windows.Query(
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
						escape("\\\\.\\"+device_name), row)
					result = append(result, virtual_directory)
				}
			}
		}
	}

	return result, nil
}

func (self *NTFSFileSystemAccessor) ReadDir(path string) ([]glob.FileInfo, error) {
	result := []glob.FileInfo{}

	path = normalize_path(path)

	// No device part, so list all devices.
	if path == "" {
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

	dir, err := self.getMFTEntry(path)
	if err != nil {
		return nil, err
	}

	// List the directory.
	for _, node := range dir.Dir() {
		node_mft_id := node.Get("mftReference").AsInteger()
		node_mft, err := dir.MFTEntry(node_mft_id)
		if err != nil {
			continue
		}

		file_name := &ntfs.FILE_NAME{node.Get("file")}

		// Skip the mft itself.
		name := file_name.Name()
		if name == "." {
			continue
		}
		result = append(result, &NTFSFileInfo{
			_name:  name,
			_isdir: node_mft.IsDir(),
			_mtime: glob.TimeVal{
				Sec: file_name.Get("file_modified").AsInteger(),
			},
			_atime: glob.TimeVal{
				Sec: file_name.Get("file_accessed").AsInteger(),
			},
			_ctime: glob.TimeVal{
				Sec: file_name.Get("created").AsInteger(),
			},
			_size:      file_name.Get("size").AsInteger(),
			_data:      vfilter.NewDict().Set("mft", node_mft_id),
			_full_path: filepath.Join(path, file_name.Name()),
		})
	}
	return result, nil
}

type readAdapter struct {
	reader io.ReaderAt
	pos    int64
}

func (self *readAdapter) Read(buf []byte) (int, error) {
	res, err := self.reader.ReadAt(buf, self.pos)
	self.pos += int64(res)

	return res, err
}

func (self *readAdapter) Close() error {
	return nil
}

func (self *readAdapter) Stat() (os.FileInfo, error) {
	return nil, errors.New("Not implementated")
}

func (self *readAdapter) Seek(offset int64, whence int) (int64, error) {
	self.pos = offset
	return self.pos, nil
}

func (self *NTFSFileSystemAccessor) Open(path string) (glob.ReadSeekCloser, error) {
	path = normalize_path(path)
	if path == "" {
		return nil, errors.New("Unable to open raw device")
	}

	mft_entry, err := self.getMFTEntry(path)
	if err != nil {
		return nil, err
	}

	// TODO Support ADS properly.
	for _, data := range mft_entry.Data() {
		return &readAdapter{reader: data.Data()}, nil
	}

	return nil, errors.New("File not found")
}

func (self *NTFSFileSystemAccessor) Lstat(filename string) (glob.FileInfo, error) {
	return glob.NewVirtualDirectoryPath("\\", nil), nil
}

// We accept both / and \ as a path separator
func (self *NTFSFileSystemAccessor) PathSep() *regexp.Regexp {
	return regexp.MustCompile("[\\\\/]")
}

// Glob sends us paths in normal form which we need to convert to
// windows form. Normal form uses / instead of \ and always has a
// leading /.
func normalize_path(path string) string {
	path = filepath.Clean(strings.Replace(path, "/", "\\", -1))
	return strings.TrimPrefix(path, "\\")

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
	profile, err := ntfs.GetProfile()
	if err == nil {
		glob.Register("ntfs", &NTFSFileSystemAccessor{profile, nil})
	}
}
