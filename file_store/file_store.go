package file_store

import (
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path"
	config "www.velocidex.com/golang/velociraptor/config"
	logging "www.velocidex.com/golang/velociraptor/logging"
)

type WriteSeekCloser interface {
	io.WriteSeeker
	io.Closer
}

type ReadSeekCloser interface {
	io.ReadSeeker
	io.Closer
}

type FileStore interface {
	ReadFile(filename string) (ReadSeekCloser, error)
	WriteFile(filename string) (WriteSeekCloser, error)
	ListDirectory(dirname string) ([]os.FileInfo, error)
}

type DirectoryFileStore struct {
	config_obj *config.Config
}

func (self *DirectoryFileStore) ListDirectory(dirname string) (
	[]os.FileInfo, error) {
	if self.config_obj.Datastore.FilestoreDirectory == "" {
		return nil, errors.New("No configured file store directory.")
	}

	file_path := path.Join(self.config_obj.Datastore.FilestoreDirectory, dirname)
	return ioutil.ReadDir(file_path)
}

func (self *DirectoryFileStore) ReadFile(filename string) (ReadSeekCloser, error) {
	if self.config_obj.Datastore.FilestoreDirectory == "" {
		return nil, errors.New("No configured file store directory.")
	}

	file_path := path.Join(self.config_obj.Datastore.FilestoreDirectory, filename)
	file, err := os.Open(file_path)
	return file, err
}

func (self *DirectoryFileStore) WriteFile(filename string) (WriteSeekCloser, error) {
	if self.config_obj.Datastore.FilestoreDirectory == "" {
		return nil, errors.New("No configured file store directory.")
	}

	file_path := path.Join(self.config_obj.Datastore.FilestoreDirectory, filename)
	err := os.MkdirAll(path.Dir(file_path), 0700)
	if err != nil {
		logging.NewLogger(self.config_obj).Error(
			"Can not create dir", err)
		return nil, err
	}

	file, err := os.OpenFile(file_path, os.O_RDWR|os.O_CREATE, 0700)
	if err != nil {
		logging.NewLogger(self.config_obj).Error(
			"Unable to open file "+file_path, err)
		return nil, err
	}

	return file, nil
}

// Currently we only support a DirectoryFileStore.
func GetFileStore(config_obj *config.Config) FileStore {
	return &DirectoryFileStore{config_obj}
}
