package file_store

// This implements a filesystem accessor which can be used to access
// the generic filestore. This allows us to run globs on the file
// store regardless of the specific filestore implementation.
import (
	"errors"
	"os"
	"time"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/vfilter"
)

type FileStoreFileSystemAccessor struct {
	file_store api.FileStore
	config_obj *config_proto.Config
}

func NewFileStoreFileSystemAccessor(
	config_obj *config_proto.Config, fs api.FileStore) *FileStoreFileSystemAccessor {
	return &FileStoreFileSystemAccessor{
		file_store: fs,
		config_obj: config_obj,
	}
}

func (self FileStoreFileSystemAccessor) New(
	scope vfilter.Scope) accessors.FileSystemAccessor {
	return &FileStoreFileSystemAccessor{
		file_store: self.file_store,
		config_obj: self.config_obj,
	}
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

	return &FileStoreFileInfo{
		FileInfo:   lstat,
		ospath:     filename,
		fullpath:   fullpath,
		config_obj: self.config_obj,
	}, nil
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
		result = append(result, NewFileStoreFileInfo(
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

func NewFileStoreFileInfo(
	config_obj *config_proto.Config,
	fullpath api.FSPathSpec,
	info os.FileInfo) *FileStoreFileInfo {

	// Create an OSPath to represent the abstract filestore path.
	// Restore the file extension from the filestore abstract
	// pathspec.
	components := utils.CopySlice(fullpath.Components())
	if len(components) > 0 {
		last_idx := len(components) - 1
		components[last_idx] += api.GetExtensionForFilestore(fullpath)
	}
	ospath := accessors.MustNewFileStorePath("fs:").Append(components...)

	return &FileStoreFileInfo{
		config_obj: config_obj,
		FileInfo:   info,
		fullpath:   fullpath,
		ospath:     ospath,
	}
}

type FileStoreFileInfo struct {
	os.FileInfo
	ospath     *accessors.OSPath
	fullpath   api.FSPathSpec
	config_obj *config_proto.Config
	Data_      *ordereddict.Dict
}

func (self FileStoreFileInfo) Name() string {
	return self.fullpath.Base()
}

func (self *FileStoreFileInfo) Data() *ordereddict.Dict {
	if self.Data_ == nil {
		return ordereddict.NewDict()
	}

	return self.Data_
}

// The FullPath contains the full URL to access the filestore.
func (self *FileStoreFileInfo) FullPath() string {
	return self.ospath.String()
}

func (self *FileStoreFileInfo) OSPath() *accessors.OSPath {
	return self.ospath
}

func (self *FileStoreFileInfo) PathSpec() api.FSPathSpec {
	return self.fullpath
}

func (self *FileStoreFileInfo) Btime() time.Time {
	return time.Time{}
}

func (self *FileStoreFileInfo) Mtime() time.Time {
	return time.Time{}
}

func (self *FileStoreFileInfo) Ctime() time.Time {
	return time.Time{}
}

func (self *FileStoreFileInfo) Atime() time.Time {
	return time.Time{}
}

func (self *FileStoreFileInfo) IsLink() bool {
	return self.Mode()&os.ModeSymlink != 0
}

// Filestores do not implementat links
func (self *FileStoreFileInfo) GetLink() (*accessors.OSPath, error) {
	return nil, errors.New("Not implemented")
}

func (self *FileStoreFileInfo) MarshalJSON() ([]byte, error) {
	result, err := json.Marshal(&struct {
		FullPath string
		Size     int64
		Mode     os.FileMode
		ModeStr  string
		ModTime  time.Time
		Btime    time.Time
		Mtime    time.Time
		Ctime    time.Time
		Atime    time.Time
	}{
		FullPath: self.FullPath(),
		Size:     self.Size(),
		Mode:     self.Mode(),
		ModeStr:  self.Mode().String(),
		ModTime:  self.ModTime(),
		Btime:    self.Btime(),
		Mtime:    self.Mtime(),
		Ctime:    self.Ctime(),
		Atime:    self.Atime(),
	})

	return result, err
}

func (self *FileStoreFileInfo) UnmarshalJSON(data []byte) error {
	return nil
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
