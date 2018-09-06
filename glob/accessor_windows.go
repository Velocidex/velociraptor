// Windows specific implementation. For windows we make a special
// virtual root directory which contains all the drives as if they are
// subdirs. For example list dir "\\" yields c:, d:, e: then we access
// each file as an absolute path: \\c:\\Windows -> c:\Windows.
package glob

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/disk"
)

type OSFileInfo struct {
	os.FileInfo

	// Empty for files but may contain data for registry and
	// resident NTFS.
	Data       string
	_full_path string
}

func (self *OSFileInfo) FullPath() string {
	return self._full_path
}

func (self *OSFileInfo) Mtime() TimeVal {
	nsec := self.sys().LastWriteTime.Nanoseconds()
	return TimeVal{
		Sec:  nsec / 1000000000,
		Nsec: nsec,
	}
}

func (self *OSFileInfo) Ctime() TimeVal {
	nsec := self.sys().CreationTime.Nanoseconds()
	return TimeVal{
		Sec:  nsec / 1000000000,
		Nsec: nsec,
	}
}

func (self *OSFileInfo) Atime() TimeVal {
	nsec := self.sys().LastAccessTime.Nanoseconds()
	return TimeVal{
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
		Mtime    TimeVal
		Ctime    TimeVal
		Atime    TimeVal
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

// Virtual FileInfo for root directory - represent all drives as
// directories.
type DrivePath struct {
	drive string
}

func (self *DrivePath) Name() string {
	return self.drive
}

func (self *DrivePath) Size() int64 {
	return 0
}

func (self *DrivePath) Mode() os.FileMode {
	return os.ModeDir
}

func (self *DrivePath) ModTime() time.Time {
	return time.Now()
}

func (self *DrivePath) IsDir() bool {
	return true
}

func (self *DrivePath) Sys() interface{} {
	return nil
}

func (self *DrivePath) FullPath() string {
	return self.drive
}

func (self *DrivePath) Atime() TimeVal {
	return TimeVal{}
}

func (self *DrivePath) Mtime() TimeVal {
	return TimeVal{}
}

func (self *DrivePath) Ctime() TimeVal {
	return TimeVal{}
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

type OSFileSystemAccessor struct{}

func (self OSFileSystemAccessor) ReadDir(path string) ([]FileInfo, error) {
	var result []FileInfo
	path = normalize_path(path)
	if path == "\\" {
		drives, err := getAvailableDrives()
		if err != nil {
			return nil, err
		}
		for _, drive := range drives {
			result = append(result, &DrivePath{drive})
		}
		return result, nil
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

func (self OSFileSystemAccessor) Open(path string) (ReadSeekCloser, error) {
	path = strings.TrimPrefix(normalize_path(path), "\\")
	// Strip leading \\ so \\c:\\windows -> c:\\windows
	file, err := os.Open(path)
	return file, err
}

func (self *OSFileSystemAccessor) Lstat(filename string) (FileInfo, error) {
	return &DrivePath{"\\"}, nil
}

// We accept both / and \ as a path separator
func (self *OSFileSystemAccessor) PathSep() *regexp.Regexp {
	return regexp.MustCompile("[\\\\/]")
}

// Glob sends us paths in normal form which we need to convert to
// windows form. Normal form uses / instead of \ and always has a
// leading /.
func normalize_path(path string) string {
	path = filepath.Clean(strings.Replace(path, "/", "\\", -1))
	return strings.TrimPrefix(path, "\\")

}

func init() {
	Register("file", &OSFileSystemAccessor{})
}
