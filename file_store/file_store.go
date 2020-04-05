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
/*

  This file store implementation stores files on disk. All of these
  functions receive serialized Velociraptor's VFS paths.

  Velociraptor paths are a sequence of string components. When the VFS
  path is serialized, we join the components using the path separator
  (by default /) . If the component contains path separators, they
  will be escaped (see utils/path.go).

  There is a direct mapping between VFS paths and filenames on
  disk. This mapping is reversible and supports correct round
  tripping.

  Use FilenameToFileStorePath() to convert from a serialized VFS path to a disk path

  Calling any of the Filestore methods (ReadDir, Open, Lstat) will
  always return VFS paths.
*/
package file_store

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	openCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "file_store_open",
		Help: "Total number of filestore open operations.",
	})

	listCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "file_store_list",
		Help: "Total number of filestore list children operations.",
	})
)

const (
	// On windows all file paths must be prefixed by this to
	// support long paths.
	WINDOWS_LFN_PREFIX = "\\\\?\\"
)

type FileReader interface {
	Read(buff []byte) (int, error)
	Seek(offset int64, whence int) (int64, error)
	Stat() (os.FileInfo, error)
	Close() error
}

// A file store writer writes files in the filestore. Filestore files
// are not as flexible as real files and only provide a subset of
// functionality. Specifically they can not be over-written - only
// appended to. They can be truncated but only to 0 size.
type FileWriter interface {
	Size() (int64, error)
	Write(data []byte) (int, error)
	Truncate() error
	Close() error
}

type FileStore interface {
	ReadFile(filename string) (FileReader, error)
	WriteFile(filename string) (FileWriter, error)
	StatFile(filename string) (*FileStoreFileInfo, error)
	ListDirectory(dirname string) ([]os.FileInfo, error)
	Walk(root string, cb filepath.WalkFunc) error
	Delete(filename string) error
}

type DirectoryFileWriter struct {
	Fd *os.File
}

func (self *DirectoryFileWriter) Size() (int64, error) {
	return self.Fd.Seek(0, os.SEEK_END)
}

func (self *DirectoryFileWriter) Write(data []byte) (int, error) {
	_, err := self.Fd.Seek(0, os.SEEK_END)
	if err != nil {
		return 0, err
	}

	return self.Fd.Write(data)
}

func (self *DirectoryFileWriter) Truncate() error {
	return self.Fd.Truncate(0)
}

func (self *DirectoryFileWriter) Close() error {
	return self.Fd.Close()
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

	listCounter.Inc()

	file_path := self.FilenameToFileStorePath(dirname)
	files, err := utils.ReadDir(file_path)
	if err != nil {
		return nil, err
	}

	var result []os.FileInfo
	for _, fileinfo := range files {
		result = append(result, &FileStoreFileInfo{
			fileinfo,
			utils.PathJoin(dirname, fileinfo.Name(), "/"),
			nil})
	}

	return result, nil
}

func getCompressed(filename string) (FileReader, error) {
	fd, err := os.Open(filename)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	zr, err := gzip.NewReader(fd)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &GzipReader{zr, fd}, nil
}

func (self *DirectoryFileStore) ReadFile(filename string) (FileReader, error) {
	file_path := self.FilenameToFileStorePath(filename)
	if strings.HasSuffix(".gz", file_path) {
		return getCompressed(file_path)
	}

	openCounter.Inc()
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

func (self *DirectoryFileStore) WriteFile(filename string) (FileWriter, error) {
	file_path := self.FilenameToFileStorePath(filename)
	err := os.MkdirAll(filepath.Dir(file_path), 0700)
	if err != nil {
		logging.GetLogger(self.config_obj,
			&logging.FrontendComponent).Error(
			"Can not create dir", err)
		return nil, err
	}

	openCounter.Inc()
	file, err := os.OpenFile(file_path, os.O_RDWR|os.O_CREATE, 0700)
	if err != nil {
		logging.GetLogger(self.config_obj, &logging.FrontendComponent).Error(
			"Unable to open file "+file_path, err)
		return nil, errors.WithStack(err)
	}

	return &DirectoryFileWriter{file}, nil
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
			filename, err_1 := self.FileStorePathToFilename(path)
			if err_1 != nil {
				return err_1
			}
			return walkFn(filename,
				&FileStoreFileInfo{info, path, nil}, err)
		})
}

var (
	mu              sync.Mutex
	implementations map[string]FileStore = make(map[string]FileStore)
)

// Currently we only support a DirectoryFileStore.
func GetFileStore(config_obj *config_proto.Config) FileStore {
	if config_obj.Datastore.Implementation == "Test" {
		mu.Lock()
		defer mu.Unlock()

		impl, pres := implementations["Test"]
		if !pres {
			impl = &MemoryFileStore{
				Data: make(map[string][]byte)}
			implementations["Test"] = impl
		}

		return impl
	}

	if config_obj.Datastore.Implementation == "MySQL" {
		res, err := NewSqlFileStore(config_obj)
		if err != nil {
			panic(err)
		}
		return res
	}

	return &DirectoryFileStore{config_obj}
}
