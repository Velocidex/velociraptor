// Windows specific implementation. For windows we make a special
// virtual root directory which contains all the drives as if they are
// subdirs. For example list dir "\\" yields c:, d:, e: then we access
// each file as an absolute path: \\c:\\Windows -> c:\Windows.
package filesystems

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/disk"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/vql/windows/wmi"
)

type OSFileInfo struct {
	os.FileInfo

	// Empty for files but may contain data for registry and
	// resident NTFS.
	_data      string
	_full_path string
}

func (self *OSFileInfo) FullPath() string {
	return self._full_path
}

func (self *OSFileInfo) Data() interface{} {
	return self._data
}

func (self *OSFileInfo) Mtime() glob.TimeVal {
	nsec := self.sys().LastWriteTime.Nanoseconds()
	return glob.TimeVal{
		Sec:  nsec / 1000000000,
		Nsec: nsec,
	}
}

func (self *OSFileInfo) Ctime() glob.TimeVal {
	nsec := self.sys().CreationTime.Nanoseconds()
	return glob.TimeVal{
		Sec:  nsec / 1000000000,
		Nsec: nsec,
	}
}

func (self *OSFileInfo) Atime() glob.TimeVal {
	nsec := self.sys().LastAccessTime.Nanoseconds()
	return glob.TimeVal{
		Sec:  nsec / 1000000000,
		Nsec: nsec,
	}
}

func (self *OSFileInfo) sys() *syscall.Win32FileAttributeData {
	return self.Sys().(*syscall.Win32FileAttributeData)
}

func (self *OSFileInfo) MarshalJSON() ([]byte, error) {
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

func (u *OSFileInfo) UnmarshalJSON(data []byte) error {
	return nil
}

func getAvailableDrives() ([]string, error) {
	partitions, err := disk.Partitions(true)
	if err != nil {
		return nil, err
	}
	result := []string{}
	for _, item := range partitions {
		// TODO: Filter only local filesystems vs. network.
		result = append(result, item.Device)
	}

	return result, nil
}

type OSFileSystemAccessor struct {
	fd_cache map[string]io.ReadCloser
}

func (self OSFileSystemAccessor) New(ctx context.Context) glob.FileSystemAccessor {
	result := &OSFileSystemAccessor{
		fd_cache: make(map[string]io.ReadCloser),
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

func discoverDriveLetters() ([]glob.FileInfo, error) {
	result := []glob.FileInfo{}

	shadow_volumes, err := wmi.Query(
		"SELECT DeviceID, Description, VolumeName, FreeSpace, "+
			"Size, SystemName, VolumeSerialNumber "+
			"from Win32_LogicalDisk",
		"ROOT\\CIMV2")
	if err == nil {
		for _, row := range shadow_volumes {
			k, pres := row.Get("DeviceID")
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

func (self OSFileSystemAccessor) ReadDir(path string) ([]glob.FileInfo, error) {
	var result []glob.FileInfo
	path = normalize_path(path)

	// No drive part, so list all drives.
	if path == "" {
		return discoverDriveLetters()
	}

	// Add a final \ to turn path into a directory path.
	path = strings.TrimPrefix(path, "\\") + "\\"
	files, err := ioutil.ReadDir(path)
	if err == nil {
		for _, f := range files {
			result = append(result,
				&OSFileInfo{
					FileInfo:   f,
					_full_path: filepath.Join(path, f.Name()),
				})
		}
		return result, nil
	}
	return nil, err
}

func (self OSFileSystemAccessor) Open(path string) (glob.ReadSeekCloser, error) {
	path = strings.TrimPrefix(normalize_path(path), "\\")
	// Strip leading \\ so \\c:\\windows -> c:\\windows
	file, err := os.Open(path)
	return file, err
}

func (self *OSFileSystemAccessor) Lstat(path string) (glob.FileInfo, error) {
	path = strings.TrimPrefix(normalize_path(path), "\\")
	// Strip leading \\ so \\c:\\windows -> c:\\windows
	stat, err := os.Lstat(path)
	return &OSFileInfo{
		FileInfo:   stat,
		_full_path: path,
	}, err
}

func (self *OSFileSystemAccessor) GetRoot(path string) (string, string, error) {
	return "/", path, nil
}

// We accept both / and \ as a path separator
func (self *OSFileSystemAccessor) PathSplit() *regexp.Regexp {
	return regexp.MustCompile("[\\\\/]")
}

func (self *OSFileSystemAccessor) PathSep() string {
	return "\\"
}

// Glob sends us paths in normal form which we need to convert to
// windows form. Normal form uses / instead of \ and always has a
// leading /.
func normalize_path(path string) string {
	path = filepath.Clean(strings.Replace(path, "/", "\\", -1))
	path = strings.TrimPrefix(path, "\\")
	if path == "." {
		return ""
	}
	return path
}

func init() {
	glob.Register("file", &OSFileSystemAccessor{})
}
