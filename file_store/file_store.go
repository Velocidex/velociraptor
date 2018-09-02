package file_store

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	config "www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/datastore"
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
	file_path, err := self.FilenameToFileStorePath(dirname)
	if err != nil {
		return nil, err
	}
	return ioutil.ReadDir(file_path)
}

func (self *DirectoryFileStore) ReadFile(filename string) (ReadSeekCloser, error) {
	file_path, err := self.FilenameToFileStorePath(filename)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(file_path)
	return file, err
}

func (self *DirectoryFileStore) WriteFile(filename string) (WriteSeekCloser, error) {
	file_path, err := self.FilenameToFileStorePath(filename)
	if err != nil {
		return nil, err
	}
	err = os.MkdirAll(path.Dir(file_path), 0700)
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

func (self *DirectoryFileStore) FilenameToFileStorePath(filename string) (
	string, error) {
	if self.config_obj.Datastore.FilestoreDirectory == "" {
		return "", errors.New("No configured file store directory.")
	}

	components := []string{self.config_obj.Datastore.FilestoreDirectory}
	for _, component := range strings.Split(filename, "/") {
		components = append(components,
			string(datastore.SanitizeString(component)))
	}

	fmt.Printf("FilestoreDirectory: %s %s\n", filename, filepath.Join(components...))

	return filepath.Join(components...), nil
}

// Currently we only support a DirectoryFileStore.
func GetFileStore(config_obj *config.Config) FileStore {
	return &DirectoryFileStore{config_obj}
}
