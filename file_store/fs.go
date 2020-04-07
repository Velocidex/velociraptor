package file_store

import (
	"net/http"
	"os"
	"strings"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

type HTTPFileAdapter struct {
	FileReader
	file_store FileStore

	filename string
}

func (self *HTTPFileAdapter) Stat() (os.FileInfo, error) {
	stat, err := self.FileReader.Stat()
	return stat, err
}

func (self HTTPFileAdapter) Readdir(count int) ([]os.FileInfo, error) {
	children, err := self.file_store.ListDirectory(self.filename)
	if err != nil {
		return nil, err
	}

	result := make([]os.FileInfo, 0, len(children))
	for _, child := range children {
		result = append(result, child)
	}

	return result, nil
}

// Implementation of http.FileSystem
type FileSystem struct {
	config_obj *config_proto.Config
	file_store FileStore

	// The required prefix of the filesystem.
	prefix string
}

func (self FileSystem) Open(path string) (http.File, error) {
	if !strings.HasPrefix(path, self.prefix) {
		return nil, os.ErrNotExist
	}

	fd, err := self.file_store.ReadFile(path)
	if err != nil {
		return nil, os.ErrNotExist
	}

	return &HTTPFileAdapter{
		FileReader: fd,
		file_store: self.file_store,
		filename:   path,
	}, nil
}

func NewFileSystem(config_obj *config_proto.Config, prefix string) *FileSystem {
	return &FileSystem{
		config_obj: config_obj,
		file_store: GetFileStore(config_obj),
		prefix:     prefix,
	}
}
