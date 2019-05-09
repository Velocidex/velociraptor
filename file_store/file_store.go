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
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	logging "www.velocidex.com/golang/velociraptor/logging"
)

type WriteSeekCloser interface {
	io.WriteSeeker
	io.Closer
	Truncate(size int64) error
}

type ReadSeekCloser interface {
	io.ReadSeeker
	io.Closer
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
}

func (self FileStoreFileInfo) Name() string {
	return datastore.UnsanitizeComponent(self.FileInfo.Name())
}

type DirectoryFileStore struct {
	config_obj *api_proto.Config
}

func (self *DirectoryFileStore) ListDirectory(dirname string) (
	[]os.FileInfo, error) {
	file_path, err := self.FilenameToFileStorePath(dirname)
	if err != nil {
		return nil, err
	}
	files, err := ioutil.ReadDir(file_path)
	if err != nil {
		return nil, err
	}

	var result []os.FileInfo
	for _, fileinfo := range files {
		result = append(result, &FileStoreFileInfo{fileinfo})
	}

	return result, nil
}

func getCompressed(filename string) (ReadSeekCloser, error) {
	fd, err := os.Open(filename)
	if err == nil {
		zr, err := gzip.NewReader(fd)
		return &SeekableGzip{zr, fd}, err
	}
	return nil, err
}

func (self *DirectoryFileStore) ReadFile(filename string) (ReadSeekCloser, error) {
	file_path, err := self.FilenameToFileStorePath(filename)
	if err != nil {
		return nil, err
	}

	if strings.HasSuffix(".gz", file_path) {
		return getCompressed(file_path)
	}

	file, err := os.Open(file_path)
	if os.IsNotExist(err) {
		return getCompressed(file_path + ".gz")
	}
	return file, err
}

func (self *DirectoryFileStore) StatFile(filename string) (*FileStoreFileInfo, error) {
	file_path, err := self.FilenameToFileStorePath(filename)
	if err != nil {
		return nil, err
	}
	file, err := os.Stat(file_path)
	if err != nil {
		return nil, err
	}

	return &FileStoreFileInfo{file}, nil
}

func (self *DirectoryFileStore) WriteFile(filename string) (WriteSeekCloser, error) {
	file_path, err := self.FilenameToFileStorePath(filename)
	if err != nil {
		return nil, err
	}

	err = os.MkdirAll(filepath.Dir(file_path), 0700)
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
		return nil, err
	}

	return file, nil
}

func (self *DirectoryFileStore) Delete(filename string) error {
	file_path, err := self.FilenameToFileStorePath(filename)
	if err != nil {
		return err
	}

	return os.Remove(file_path)
}

func (self *DirectoryFileStore) FilenameToFileStorePath(filename string) (
	string, error) {
	if self.config_obj.Datastore.FilestoreDirectory == "" {
		return "", errors.New("no configured file store directory")
	}

	components := []string{self.config_obj.Datastore.FilestoreDirectory}
	for idx, component := range strings.Split(filename, "/") {
		if idx == 0 && component == "aff4:" {
			continue
		}

		component = strings.Replace(component, "\\", "/", -1)

		components = append(components,
			string(datastore.SanitizeString(component)))
	}

	return filepath.Join(components...), nil
}

func (self *DirectoryFileStore) FileStorePathToFilename(filename string) (
	string, error) {
	if self.config_obj.Datastore.FilestoreDirectory == "" {
		return "", errors.New("no configured file store directory")
	}

	if !strings.HasPrefix(filename, self.config_obj.Datastore.FilestoreDirectory) {
		return "", errors.New("not a file store directory")
	}

	components := []string{}
	for _, component := range strings.Split(
		strings.TrimPrefix(
			filename, self.config_obj.Datastore.FilestoreDirectory),
		string(os.PathSeparator)) {
		components = append(components,
			string(datastore.UnsanitizeComponent(component)))
	}

	result := filepath.Join(components...)

	// This relies on the filepath starting with a drive letter
	// and having \ as path separators. Main's
	// validateServerConfig() ensures this is the case.
	if runtime.GOOS == "windows" {
		return "\\\\?\\" + result, nil
	}

	return result, nil
}

func (self *DirectoryFileStore) Walk(root string, walkFn filepath.WalkFunc) error {
	path, err := self.FilenameToFileStorePath(root)
	if err != nil {
		return err
	}

	return filepath.Walk(path,
		func(path string, info os.FileInfo, err error) error {
			filestore_path, _ := self.FileStorePathToFilename(path)
			return walkFn(filestore_path,
				&FileStoreFileInfo{info}, err)
		})
}

// Currently we only support a DirectoryFileStore.
func GetFileStore(config_obj *api_proto.Config) FileStore {
	return &DirectoryFileStore{config_obj}
}
