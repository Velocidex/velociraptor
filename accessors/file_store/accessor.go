package file_store

// This implements a filesystem accessor which can be used to access
// the generic filestore. This allows us to run globs on the file
// store regardless of the specific filestore implementation.
import (
	"errors"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/file_store_file_info"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type FileStoreFileSystemAccessor struct {
	file_store api.FileStore
	config_obj *config_proto.Config
}

func NewFileStoreFileSystemAccessor(config_obj *config_proto.Config) *FileStoreFileSystemAccessor {
	return &FileStoreFileSystemAccessor{
		file_store: file_store.GetFileStore(config_obj),
		config_obj: config_obj,
	}
}

func (self FileStoreFileSystemAccessor) New(
	scope vfilter.Scope) (accessors.FileSystemAccessor, error) {
	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		return &FileStoreFileSystemAccessor{
			file_store: self.file_store,
			config_obj: self.config_obj,
		}, nil
	}

	return NewFileStoreFileSystemAccessor(config_obj), nil
}

func (self FileStoreFileSystemAccessor) Lstat(filename string) (
	accessors.FileInfo, error) {
	full_path, err := self.ParsePath(filename)
	if err != nil {
		return nil, err
	}

	return self.LstatWithOSPath(full_path)
}

func (self FileStoreFileSystemAccessor) LstatWithOSPath(filename *accessors.OSPath) (
	accessors.FileInfo, error) {

	fullpath := getFSPathSpec(filename)
	lstat, err := self.file_store.StatFile(fullpath)
	if err != nil {
		return nil, err
	}

	return file_store_file_info.NewFileStoreFileInfoWithOSPath(
		self.config_obj, filename, fullpath, lstat), nil
}

func (self FileStoreFileSystemAccessor) ParsePath(path string) (
	*accessors.OSPath, error) {
	return accessors.NewFileStorePath(path)
}

func (self FileStoreFileSystemAccessor) ReadDir(filename string) (
	[]accessors.FileInfo, error) {
	full_path, err := self.ParsePath(filename)
	if err != nil {
		return nil, err
	}

	return self.ReadDirWithOSPath(full_path)
}

func (self FileStoreFileSystemAccessor) ReadDirWithOSPath(
	filename *accessors.OSPath) (
	[]accessors.FileInfo, error) {

	fullpath := getFSPathSpec(filename)
	files, err := self.file_store.ListDirectory(fullpath)
	if err != nil {
		return nil, err
	}

	var result []accessors.FileInfo
	for _, f := range files {
		result = append(result, file_store_file_info.NewFileStoreFileInfo(
			self.config_obj, f.PathSpec(), f))
	}

	return result, nil
}

func (self FileStoreFileSystemAccessor) Open(filename string) (
	accessors.ReadSeekCloser, error) {
	full_path, err := self.ParsePath(filename)
	if err != nil {
		return nil, err
	}

	return self.OpenWithOSPath(full_path)
}

func (self FileStoreFileSystemAccessor) OpenWithOSPath(filename *accessors.OSPath) (
	accessors.ReadSeekCloser, error) {

	if len(filename.Components) == 0 {
		return nil, errors.New("Invalid path")
	}

	// It is a data store path
	if filename.Components[0] == "ds:" {
		ds_path := getDSPathSpec(filename)
		fullpath := ds_path.AsFilestorePath()
		switch ds_path.Type() {
		case api.PATH_TYPE_DATASTORE_JSON:
			fullpath = fullpath.SetType(api.PATH_TYPE_FILESTORE_DB_JSON)

		case api.PATH_TYPE_DATASTORE_PROTO:
			fullpath = fullpath.SetType(api.PATH_TYPE_FILESTORE_DB)
		}
	}

	fullpath := getFSPathSpec(filename)
	file, err := self.file_store.ReadFile(fullpath)
	if err != nil {
		return nil, err
	}

	return file, nil
}

func getFSPathSpec(filename *accessors.OSPath) api.FSPathSpec {
	result := path_specs.NewUnsafeFilestorePath(filename.Components...)
	if len(result.Components()) > 0 {
		last := len(filename.Components) - 1
		name_type, name := api.GetFileStorePathTypeFromExtension(
			filename.Components[last])
		filename.Components[last] = name
		return result.SetType(name_type)
	}
	return result
}

func getDSPathSpec(filename *accessors.OSPath) api.DSPathSpec {
	result := path_specs.NewUnsafeDatastorePath(filename.Components...)
	if len(filename.Components) > 0 {
		last := len(filename.Components) - 1
		name_type, name := api.GetDataStorePathTypeFromExtension(
			filename.Components[last])
		filename.Components[last] = name
		return result.SetType(name_type)
	}
	return result
}
