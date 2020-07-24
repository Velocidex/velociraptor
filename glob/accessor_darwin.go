// +build darwin

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
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type OSFileInfo struct {
	os.FileInfo
	_full_path string
}

func (self *OSFileInfo) FullPath() string {
	return self._full_path
}

func (self *OSFileInfo) Mtime() utils.TimeVal {
	return utils.TimeVal{
		Sec:  int64(self.sys().Mtimespec.Sec),
		Nsec: int64(self.sys().Mtimespec.Nsec),
	}
}

func (self *OSFileInfo) Ctime() utils.TimeVal {
	return utils.TimeVal{
		Sec:  int64(self.sys().Ctimespec.Sec),
		Nsec: int64(self.sys().Ctimespec.Nsec),
	}
}

func (self *OSFileInfo) Atime() utils.TimeVal {
	return utils.TimeVal{
		Sec:  int64(self.sys().Atimespec.Sec),
		Nsec: int64(self.sys().Atimespec.Nsec),
	}
}

func (self *OSFileInfo) Data() interface{} {
	return nil
}

func (self *OSFileInfo) sys() *syscall.Stat_t {
	return self.Sys().(*syscall.Stat_t)
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

func (self *OSFileInfo) MarshalJSON() ([]byte, error) {
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

func (u *OSFileInfo) UnmarshalJSON(data []byte) error {
	return nil
}

// Real implementation for non windows OSs:
type OSFileSystemAccessor struct{}

func (self OSFileSystemAccessor) New(scope *vfilter.Scope) (FileSystemAccessor, error) {
	result := &OSFileSystemAccessor{}
	return result, nil
}

func (self OSFileSystemAccessor) Lstat(filename string) (FileInfo, error) {
	lstat, err := os.Lstat(GetPath(filename))
	if err != nil {
		return nil, err
	}

	return &OSFileInfo{lstat, filename}, nil
}

func (self OSFileSystemAccessor) ReadDir(path string) ([]FileInfo, error) {
	files, err := ioutil.ReadDir(GetPath(path))
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
	file, err := os.Open(GetPath(path))
	return file, err
}

func GetPath(path string) string {
	return filepath.Clean("/" + path)
}

var OSFileSystemAccessor_re = regexp.MustCompile("/")

func (self OSFileSystemAccessor) PathSplit(path string) []string {
	return OSFileSystemAccessor_re.Split(path, -1)
}

func (self OSFileSystemAccessor) PathJoin(root, stem string) string {
	return filepath.Join(root, stem)
}

func (self OSFileSystemAccessor) GetRoot(path string) (string, string, error) {
	return "/", path, nil
}

func init() {
	Register("file", &OSFileSystemAccessor{})
	Register("auto", &OSFileSystemAccessor{})
}
