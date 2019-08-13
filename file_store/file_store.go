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
package file_store

import (
	"compress/gzip"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pkg/errors"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	// On windows all file paths must be prefixed by this to
	// support long paths.
	WINDOWS_LFN_PREFIX = "\\\\?\\"
)

type WriteSeekCloser interface {
	io.WriteSeeker
	io.Closer
	Truncate(size int64) error
}

type ReadSeekCloser interface {
	io.ReadSeeker
	io.Closer

	Stat() (os.FileInfo, error)
}

type FileStore interface {
	ReadFile(filename string) (ReadSeekCloser, error)
	WriteFile(filename string) (WriteSeekCloser, error)
	StatFile(filename string) (*FileStoreFileInfo, error)
	ListDirectory(dirname string) ([]os.FileInfo, error)
	Walk(root string, cb filepath.WalkFunc) error
	Delete(filename string) error
}

type FileStoreFileInfo struct {
	os.FileInfo
	_full_path string
	_data      interface{}
}

func (self FileStoreFileInfo) Name() string {
	return datastore.UnsanitizeComponent(self.FileInfo.Name())
}

type DirectoryFileStore struct {
	config_obj *config_proto.Config
}

func (self *DirectoryFileStore) ListDirectory(dirname string) (
	[]os.FileInfo, error) {
	file_path := self.FilenameToFileStorePath(dirname)
	files, err := ioutil.ReadDir(file_path)
	if err != nil {
		return nil, err
	}

	var result []os.FileInfo
	for _, fileinfo := range files {
		result = append(result, &FileStoreFileInfo{
			fileinfo, dirname, nil})
	}

	return result, nil
}

func getCompressed(filename string) (ReadSeekCloser, error) {
	fd, err := os.Open(filename)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	zr, err := gzip.NewReader(fd)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &SeekableGzip{zr, fd}, nil
}

func (self *DirectoryFileStore) ReadFile(filename string) (ReadSeekCloser, error) {
	file_path := self.FilenameToFileStorePath(filename)
	if strings.HasSuffix(".gz", file_path) {
		return getCompressed(file_path)
	}

	file, err := os.Open(file_path)
	if os.IsNotExist(err) {
		return getCompressed(file_path + ".gz")
	}

	if err != nil {
		return nil, errors.WithStack(err)
	}
	return file, nil
}

func (self *DirectoryFileStore) StatFile(filename string) (*FileStoreFileInfo, error) {
	file_path := self.FilenameToFileStorePath(filename)
	file, err := os.Stat(file_path)
	if err != nil {
		return nil, err
	}

	return &FileStoreFileInfo{file, filename, nil}, nil
}

func (self *DirectoryFileStore) WriteFile(filename string) (WriteSeekCloser, error) {
	file_path := self.FilenameToFileStorePath(filename)
	err := os.MkdirAll(filepath.Dir(file_path), 0700)
	if err != nil {
		logging.GetLogger(self.config_obj,
			&logging.FrontendComponent).Error(
			"Can not create dir", err)
		return nil, err
	}

	file, err := os.OpenFile(file_path, os.O_RDWR|os.O_CREATE, 0700)
	if err != nil {
		logging.GetLogger(self.config_obj, &logging.FrontendComponent).Error(
			"Unable to open file "+file_path, err)
		return nil, errors.WithStack(err)
	}

	return file, nil
}

func (self *DirectoryFileStore) Delete(filename string) error {
	file_path := self.FilenameToFileStorePath(filename)
	return os.Remove(file_path)
}

// In the below:
// Filename: is an abstract filename to be represented in the file store.
// FileStorePath: An actual path to store the file on the filesystem.
//
// On windows, the FileStorePath always includes the LFN prefix.
func (self *DirectoryFileStore) FilenameToFileStorePath(filename string) string {
	components := []string{self.config_obj.Datastore.FilestoreDirectory}
	for _, component := range utils.SplitComponents(filename) {
		components = append(components,
			string(datastore.SanitizeString(component)))
	}

	result := filepath.Join(components...)
	if runtime.GOOS == "windows" {
		return WINDOWS_LFN_PREFIX + result
	}
	return result
}

// Converts from a physical path on disk to a normalized filestore path.
func (self *DirectoryFileStore) FileStorePathToFilename(filename string) (
	string, error) {

	if runtime.GOOS == "windows" {
		filename = strings.TrimPrefix(filename, WINDOWS_LFN_PREFIX)
	}
	filename = strings.TrimPrefix(filename,
		self.config_obj.Datastore.FilestoreDirectory)

	components := []string{}
	for _, component := range strings.Split(
		filename,
		string(os.PathSeparator)) {
		components = append(components,
			string(datastore.UnsanitizeComponent(component)))
	}

	result := filepath.Join(components...)
	return result, nil
}

func (self *DirectoryFileStore) Walk(root string, walkFn filepath.WalkFunc) error {
	path := self.FilenameToFileStorePath(root)
	return filepath.Walk(path,
		func(path string, info os.FileInfo, err error) error {
			filename, err := self.FileStorePathToFilename(path)
			if err != nil {
				return err
			}
			return walkFn(filename,
				&FileStoreFileInfo{info, path, nil}, err)
		})
}

// Currently we only support a DirectoryFileStore.
func GetFileStore(config_obj *config_proto.Config) FileStore {
	return &DirectoryFileStore{config_obj}
}
