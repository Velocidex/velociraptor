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
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"github.com/Velocidex/ordereddict"
	errors "github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type OSFileInfo struct {
	_FileInfo  os.FileInfo
	_full_path string
	_data      interface{}
}

func (self *OSFileInfo) Size() int64 {
	return self._FileInfo.Size()
}

func (self *OSFileInfo) Name() string {
	return self._FileInfo.Name()
}

func (self *OSFileInfo) IsDir() bool {
	return self._FileInfo.IsDir()
}

func (self *OSFileInfo) ModTime() time.Time {
	return self._FileInfo.ModTime()
}

func (self *OSFileInfo) Mode() os.FileMode {
	return self._FileInfo.Mode()
}

func (self *OSFileInfo) Sys() interface{} {
	return self._FileInfo.Sys()
}

func (self *OSFileInfo) Data() interface{} {
	if self._data == nil {
		return ordereddict.NewDict()
	}
	return self._data
}

func (self *OSFileInfo) FullPath() string {
	return self._full_path
}

func (self *OSFileInfo) Mtime() utils.TimeVal {
	ts := int64(self._Sys().Mtim.Sec)
	return utils.TimeVal{
		Sec:  ts,
		Nsec: int64(self._Sys().Mtim.Nsec) + ts*1000000000,
	}
}

func (self *OSFileInfo) Ctime() utils.TimeVal {
	ts := int64(self._Sys().Ctim.Sec)
	return utils.TimeVal{
		Sec:  ts,
		Nsec: int64(self._Sys().Ctim.Nsec) + ts*1000000000,
	}
}

func (self *OSFileInfo) Atime() utils.TimeVal {
	ts := int64(self._Sys().Atim.Sec)
	return utils.TimeVal{
		Sec:  ts,
		Nsec: int64(self._Sys().Atim.Nsec) + ts*1000000000,
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

	return "", errors.New("Links not supported")
	return target, nil
}

func (self *OSFileInfo) _Sys() *syscall.Stat_t {
	return self._FileInfo.Sys().(*syscall.Stat_t)
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

func (self *OSFileInfo) UnmarshalJSON(data []byte) error {
	return nil
}

// Real implementation for non windows OSs:
type OSFileSystemAccessor struct{}

func (self OSFileSystemAccessor) New(scope *vfilter.Scope) (FileSystemAccessor, error) {
	return &OSFileSystemAccessor{}, nil
}

func (self OSFileSystemAccessor) Lstat(filename string) (FileInfo, error) {
	lstat, err := os.Lstat(GetPath(filename))
	if err != nil {
		return nil, err
	}

	return &OSFileInfo{lstat, filename, nil}, nil
}

func (self OSFileSystemAccessor) ReadDir(path string) ([]FileInfo, error) {
	files, err := utils.ReadDir(GetPath(path))
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
	file, err := os.Open(GetPath(path))
	if err != nil {
		return nil, err
	}

	return file, nil
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

func (self *OSFileSystemAccessor) GetRoot(path string) (string, string, error) {
	return "/", path, nil
}

func init() {
	Register("file", &OSFileSystemAccessor{})

	// On Linux the auto accessor is the same as file.
	Register("auto", &OSFileSystemAccessor{})
}
