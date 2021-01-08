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
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Velocidex/ordereddict"
	errors "github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type _inode struct {
	dev, inode uint64
}

// Keep track of symlinks we visited.
type AccessorContext struct {
	mu sync.Mutex

	links map[_inode]bool
}

func (self *AccessorContext) LinkVisited(dev, inode uint64) {
	id := _inode{dev, inode}

	self.mu.Lock()
	defer self.mu.Unlock()

	self.links[id] = true
}

func (self *AccessorContext) WasLinkVisited(dev, inode uint64) bool {
	id := _inode{dev, inode}

	self.mu.Lock()
	defer self.mu.Unlock()

	_, pres := self.links[id]
	return pres
}

type OSFileInfo struct {
	_FileInfo     os.FileInfo
	_full_path    string
	_accessor_ctx *AccessorContext
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
	if self.IsLink() {
		path := strings.TrimRight(
			strings.TrimLeft(self.FullPath(), "\\"), "\\")
		target, err := os.Readlink(path)
		if err == nil {
			return ordereddict.NewDict().
				Set("Link", target)
		}
	}

	return ordereddict.NewDict()
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
	sys, ok := self._FileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return "", errors.New("Symlink not supported")
	}

	if self._accessor_ctx.WasLinkVisited(sys.Dev, sys.Ino) {
		return "", errors.New("Symlink cycle detected")
	}
	self._accessor_ctx.LinkVisited(sys.Dev, sys.Ino)

	// For now we dont support links so we dont get stuck in a
	// cycle.
	ret, err := os.Readlink(strings.TrimRight(self._full_path, "/"))
	if err != nil {
		return "", err
	}

	if !strings.HasPrefix(ret, "/") {
		ret = "/" + ret
	}

	return ret, nil
}

func (self *OSFileInfo) _Sys() *syscall.Stat_t {
	return self._FileInfo.Sys().(*syscall.Stat_t)
}

// Real implementation for non windows OSs:
type OSFileSystemAccessor struct {
	context *AccessorContext
}

func (self OSFileSystemAccessor) New(scope vfilter.Scope) (FileSystemAccessor, error) {
	return &OSFileSystemAccessor{
		context: &AccessorContext{
			links: make(map[_inode]bool),
		},
	}, nil
}

func (self OSFileSystemAccessor) Lstat(filename string) (FileInfo, error) {
	lstat, err := os.Lstat(GetPath(filename))
	if err != nil {
		return nil, err
	}

	return &OSFileInfo{
		_FileInfo:     lstat,
		_full_path:    filename,
		_accessor_ctx: self.context,
	}, nil
}

func (self OSFileSystemAccessor) ReadDir(path string) ([]FileInfo, error) {
	path = GetPath(path)

	lstat, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}

	// Support symlinks and directories.
	if lstat.Mode()&os.ModeSymlink == 0 && !lstat.IsDir() {
		return nil, nil
	}

	files, err := utils.ReadDir(GetPath(path))
	if err != nil {
		return nil, err
	}

	var result []FileInfo
	for _, f := range files {
		result = append(result,
			&OSFileInfo{
				_FileInfo:     f,
				_full_path:    filepath.Join(path, f.Name()),
				_accessor_ctx: self.context,
			})
	}

	return result, nil
}

func (self OSFileSystemAccessor) Open(path string) (ReadSeekCloser, error) {
	var err error

	// Eval any symlinks directly
	path, err = filepath.EvalSymlinks(GetPath(path))
	if err != nil {
		return nil, err
	}

	lstat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if !lstat.Mode().IsRegular() {
		return nil, errors.New("Only regular files supported")
	}

	file, err := os.Open(path)
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

	json.RegisterCustomEncoder(&OSFileInfo{}, MarshalGlobFileInfo)
}
