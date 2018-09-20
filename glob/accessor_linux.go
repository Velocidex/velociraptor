// +build linux

package glob

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"www.velocidex.com/golang/velociraptor/utils"
)

type OSFileInfo struct {
	os.FileInfo
	_full_path string
	_data      interface{}
}

func (self *OSFileInfo) Data() interface{} {
	return self._data
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
type OSFileSystemAccessor struct {
	fd_cache map[string]*os.File
}

func (self OSFileSystemAccessor) New(ctx context.Context) FileSystemAccessor {
	result := &OSFileSystemAccessor{
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

func (self OSFileSystemAccessor) Lstat(filename string) (FileInfo, error) {
	lstat, err := os.Lstat(filename)
	if err != nil {
		return nil, err
	}

	return &OSFileInfo{lstat, filename, nil}, nil
}

func (self OSFileSystemAccessor) ReadDir(path string) ([]FileInfo, error) {
	files, err := utils.ReadDir(path)
	if files != nil {
		var result []FileInfo
		for _, f := range files {
			result = append(result,
				&OSFileInfo{f, filepath.Join(path, f.Name()), nil})
		}

		return result, nil
	}
	return nil, err
}

func (self OSFileSystemAccessor) Open(path string) (ReadSeekCloser, error) {
	fd, pres := self.fd_cache[path]
	if pres {
		return fd, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	self.fd_cache[path] = file

	return file, nil
}

func (self OSFileSystemAccessor) PathSep() *regexp.Regexp {
	return regexp.MustCompile("/")
}

func init() {
	Register("file", &OSFileSystemAccessor{})
}
