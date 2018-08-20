// +build !windows

package glob

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
	"time"
)

type OSFileInfo struct {
	os.FileInfo
	_full_path string
}

func (self *OSFileInfo) FullPath() string {
	return self._full_path
}

func (self *OSFileInfo) Mtime() TimeVal {
	return TimeVal{
		Sec:  self.sys().Mtim.Sec,
		Nsec: self.sys().Mtim.Nsec,
	}
}

func (self *OSFileInfo) Ctime() TimeVal {
	return TimeVal{
		Sec:  self.sys().Ctim.Sec,
		Nsec: self.sys().Ctim.Nsec,
	}
}

func (self *OSFileInfo) Atime() TimeVal {
	return TimeVal{
		Sec:  self.sys().Atim.Sec,
		Nsec: self.sys().Atim.Nsec,
	}
}

func (self *OSFileInfo) sys() *syscall.Stat_t {
	return self.Sys().(*syscall.Stat_t)
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

// Real implementation for non windows OSs:
type OSFileSystemAccessor struct{}

func (self OSFileSystemAccessor) Lstat(filename string) (FileInfo, error) {
	lstat, err := os.Lstat(filename)
	if err != nil {
		return nil, err
	}

	return &OSFileInfo{lstat, filename}, nil
}

func (self OSFileSystemAccessor) ReadDir(path string) ([]FileInfo, error) {
	files, err := ioutil.ReadDir(path)
	if err == nil {
		var result []FileInfo
		for _, f := range files {
			result = append(result,
				&OSFileInfo{f, filepath.Join(path, f.Name())})
		}
		return result, nil
	}
	return nil, err
}

func (self OSFileSystemAccessor) Open(path string) (ReadSeekCloser, error) {
	file, err := os.Open(path)
	return file, err
}

func (self OSFileSystemAccessor) PathSep() *regexp.Regexp {
	return regexp.MustCompile("/")
}

func init() {
	Register("file", &OSFileSystemAccessor{})
}
