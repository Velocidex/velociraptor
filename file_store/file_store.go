package file_store

import (
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
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

func (self *DirectoryFileStore) ReadFile(filename string) (ReadSeekCloser, error) {
	file_path, err := self.FilenameToFileStorePath(filename)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(file_path)
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

	return filepath.Join(components...), nil
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
