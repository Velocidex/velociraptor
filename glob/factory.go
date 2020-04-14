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
	"path/filepath"
	"regexp"

	errors "github.com/pkg/errors"
	"www.velocidex.com/golang/vfilter"
)

var (
	handlers map[string]FileSystemAccessor
)

// Interface for accessing the filesystem. Used for dependency
// injection.
type FileSystemAccessor interface {
	ReadDir(path string) ([]FileInfo, error)
	Open(path string) (ReadSeekCloser, error)
	Lstat(filename string) (FileInfo, error)

	// Produce a function which splits a path into components.
	PathSplit(path string) []string
	PathJoin(root, stem string) string

	// Split a path into a glob root and a sub path
	// component. This is required when the accessor uses a prefix
	// which contains path separators but should not be considered
	// as part of the glob. For example, the ntfs accessor uses a
	// device path as the root, which already contains path
	// separators:
	// \\.\c:\Windows\System32\notepad.exe ->
	// root = \\.\c:
	// path = \Windows\System32\notepad.exe
	GetRoot(path string) (root, subpath string, err error)

	// A factory for new accessors
	New(scope *vfilter.Scope) FileSystemAccessor
}

type NullFileSystemAccessor struct{}

func (self NullFileSystemAccessor) New(scope *vfilter.Scope) FileSystemAccessor {
	return self
}

func (self NullFileSystemAccessor) ReadDir(path string) ([]FileInfo, error) {
	return nil, errors.New("Not supported")
}

func (self NullFileSystemAccessor) Open(path string) (ReadSeekCloser, error) {
	return nil, errors.New("Not supported")
}

func (self NullFileSystemAccessor) Lstat(path string) (FileInfo, error) {
	return nil, errors.New("Not supported")
}

func (self NullFileSystemAccessor) GetRoot(path string) (string, string, error) {
	return "/", path, nil
}

func (self NullFileSystemAccessor) PathSplit(path string) []string {
	re := regexp.MustCompile("/")
	return re.Split(path, -1)
}

func (self NullFileSystemAccessor) PathJoin(root, stem string) string {
	return filepath.Join(root, stem)
}

func GetAccessor(scheme string, scope *vfilter.Scope) (
	FileSystemAccessor, error) {

	// Fallback to the file handler - this should work
	// because there needs to be at least a file handler
	// registered.
	if scheme == "" {
		scheme = "file"
	}

	handler, pres := handlers[scheme]
	if pres {
		return handler.New(scope), nil
	}

	return nil, errors.New("Unknown filesystem accessor")
}

func Register(scheme string, accessor FileSystemAccessor) {
	if handlers == nil {
		handlers = make(map[string]FileSystemAccessor)
	}

	handlers[scheme] = accessor
}
