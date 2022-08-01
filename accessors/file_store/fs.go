package file_store

import (
	"net/http"
	"os"
	"strings"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/utils"
)

type HTTPFileAdapter struct {
	api.FileReader
	file_store api.FileStore

	filename api.FSPathSpec
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
	for _, i := range children {
		result = append(result, i)
	}

	return result, nil
}

// Implementation of http.FileSystem
type FileSystem struct {
	config_obj *config_proto.Config
	file_store api.FileStore

	// The required prefix of the filesystem.
	prefix string
}

func (self FileSystem) Open(path string) (http.File, error) {
	if !strings.HasPrefix(path, self.prefix) {
		return nil, os.ErrNotExist
	}

	components := path_specs.NewUnsafeFilestorePath(
		utils.SplitComponents(path)...).
		SetType(api.PATH_TYPE_FILESTORE_ANY)
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
	file_store api.FileStore,
	prefix string) *FileSystem {
	return &FileSystem{
		config_obj: config_obj,
		file_store: file_store,
		prefix:     prefix,
	}
}
