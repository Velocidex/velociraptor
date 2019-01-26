// +build linux

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
		Sec:  int64(self.sys().Mtim.Sec),
		Nsec: int64(self.sys().Mtim.Nsec),
	}
}

func (self *OSFileInfo) Ctime() TimeVal {
	return TimeVal{
		Sec:  int64(self.sys().Ctim.Sec),
		Nsec: int64(self.sys().Ctim.Nsec),
	}
}

func (self *OSFileInfo) Atime() TimeVal {
	return TimeVal{
		Sec:  int64(self.sys().Atim.Sec),
		Nsec: int64(self.sys().Atim.Nsec),
	}
}

func (self *OSFileInfo) IsLink() bool {
	return self.Mode()&os.ModeSymlink != 0
}

func (self *OSFileInfo) GetLink() (string, error) {
	target, err := os.Readlink(self._full_path)
	if err != nil {
		return "", err
	}
	return target, nil
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

func (self *OSFileInfo) UnmarshalJSON(data []byte) error {
	return nil
}

// Real implementation for non windows OSs:
type OSFileSystemAccessor struct{}

func (self OSFileSystemAccessor) New(ctx context.Context) FileSystemAccessor {
	result := &OSFileSystemAccessor{}
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
	if err != nil {
		return nil, err
	}

	var result []FileInfo
	for _, f := range files {
		result = append(result,
			&OSFileInfo{f, filepath.Join(path, f.Name()), nil})
	}

	return result, nil
}

func (self OSFileSystemAccessor) Open(path string) (ReadSeekCloser, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	return file, nil
}

func (self OSFileSystemAccessor) PathSplit() *regexp.Regexp {
	return regexp.MustCompile("/")
}

func (self *OSFileSystemAccessor) PathSep() string {
	return "/"
}

func (self *OSFileSystemAccessor) GetRoot(path string) (string, string, error) {
	return "/", path, nil
}

func init() {
	Register("file", &OSFileSystemAccessor{})
}
