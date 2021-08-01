package api

import (
	"net/http"
	"os"
	"strings"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

type HTTPFileAdapter struct {
	FileReader
	file_store FileStore

	filename SafeDatastorePath
}

func (self *HTTPFileAdapter) Stat() (os.FileInfo, error) {
	stat, err := self.FileReader.Stat()
	return stat, err
}

func (self HTTPFileAdapter) Readdir(count int) ([]os.FileInfo, error) {
	return self.file_store.ListDirectory(self.filename)
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

	components := NewSafeDatastorePath(utils.SplitComponents(path)...)
	fd, err := self.file_store.ReadFile(components)
	if err != nil {
		return nil, os.ErrNotExist
	}

	return &HTTPFileAdapter{
		FileReader: fd,
		file_store: self.file_store,
		filename:   components,
	}, nil
}

func NewFileSystem(
	config_obj *config_proto.Config,
	file_store FileStore,
	prefix string) *FileSystem {
	return &FileSystem{
		config_obj: config_obj,
		file_store: file_store,
		prefix:     prefix,
	}
}
