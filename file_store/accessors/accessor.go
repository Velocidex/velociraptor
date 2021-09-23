package accessors

// This implements a filesystem accessor which can be used to access
// the generic filestore. This allows us to run globs on the file
// store regardless of the specific filestore implementation.  This
// accessor is for DirectoryFileStore

import (
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/glob"
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
	scope vfilter.Scope) glob.FileSystemAccessor {
	return &FileStoreFileSystemAccessor{
		file_store: self.file_store,
		config_obj: self.config_obj,
	}
}

func (self FileStoreFileSystemAccessor) Lstat(
	filename string) (glob.FileInfo, error) {

	fullpath := getFSPathSpec(filename)
	lstat, err := self.file_store.StatFile(fullpath)
	if err != nil {
		return nil, err
	}

	return &FileStoreFileInfo{
		FileInfo:   lstat,
		config_obj: self.config_obj,
		fullpath:   fullpath,
	}, nil
}

func (self FileStoreFileSystemAccessor) ReadDir(filename string) (
	[]glob.FileInfo, error) {
	fullpath := getFSPathSpec(filename)
	files, err := self.file_store.ListDirectory(fullpath)
	if err != nil {
		return nil, err
	}

	var result []glob.FileInfo
	for _, f := range files {
		result = append(result,
			&FileStoreFileInfo{
				FileInfo:   f,
				fullpath:   f.PathSpec(),
				config_obj: self.config_obj,
			})
	}

	return result, nil
}

func (self FileStoreFileSystemAccessor) Open(filename string) (
	glob.ReadSeekCloser, error) {

	fullpath := getFSPathSpec(filename)
	if strings.HasPrefix(filename, "ds:") {
		ds_path := getDSPathSpec(filename)
		fullpath = ds_path.AsFilestorePath()
		switch ds_path.Type() {
		case api.PATH_TYPE_DATASTORE_JSON:
			fullpath = fullpath.SetType(api.PATH_TYPE_FILESTORE_DB_JSON)

		case api.PATH_TYPE_DATASTORE_PROTO:
			fullpath = fullpath.SetType(api.PATH_TYPE_FILESTORE_DB)
		}
	}

	file, err := self.file_store.ReadFile(fullpath)
	if err != nil {
		return nil, err
	}

	return &FileReaderAdapter{file}, nil
}

var FileStoreFileSystemAccessor_re = regexp.MustCompile("/")

func (self FileStoreFileSystemAccessor) PathSplit(path string) []string {
	return FileStoreFileSystemAccessor_re.Split(path, -1)
}

func (self FileStoreFileSystemAccessor) PathJoin(root, stem string) string {
	return path.Join(root, stem)
}

func (self *FileStoreFileSystemAccessor) GetRoot(path string) (string, string, error) {
	return "/", path, nil
}

func NewFileStoreFileInfo(
	config_obj *config_proto.Config,
	fullpath api.FSPathSpec,
	info os.FileInfo) *FileStoreFileInfo {
	return &FileStoreFileInfo{
		config_obj: config_obj,
		FileInfo:   info,
		fullpath:   fullpath,
	}
}

type FileStoreFileInfo struct {
	os.FileInfo
	fullpath   api.FSPathSpec
	config_obj *config_proto.Config
	Data_      interface{}
}

func (self FileStoreFileInfo) Name() string {
	return self.fullpath.Base()
}

func (self *FileStoreFileInfo) Data() interface{} {
	if self.Data_ == nil {
		return ordereddict.NewDict()
	}

	return self.Data_
}

// The FullPath contains the full URL to access the filestore.
func (self *FileStoreFileInfo) FullPath() string {
	return "fs:" + self.fullpath.AsClientPath()
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

func (self *FileStoreFileInfo) GetLink() (string, error) {
	filename := self.fullpath.AsFilestoreFilename(self.config_obj)
	target, err := os.Readlink(filename)
	if err != nil {
		return "", err
	}
	return target, nil
}

func (self *FileStoreFileInfo) MarshalJSON() ([]byte, error) {
	result, err := json.Marshal(&struct {
		FullPath string
		Size     int64
		Mode     os.FileMode
		ModeStr  string
		ModTime  time.Time
		Sys      interface{}
		Mtime    time.Time
		Ctime    time.Time
		Atime    time.Time
	}{
		FullPath: self.FullPath(),
		Size:     self.Size(),
		Mode:     self.Mode(),
		ModeStr:  self.Mode().String(),
		ModTime:  self.ModTime(),
		Sys:      self.Sys(),
		Mtime:    self.Mtime(),
		Ctime:    self.Ctime(),
		Atime:    self.Atime(),
	})

	return result, err
}

func (self *FileStoreFileInfo) UnmarshalJSON(data []byte) error {
	return nil
}

type FileReaderAdapter struct {
	api.FileReader
}

func (self *FileReaderAdapter) Stat() (os.FileInfo, error) {
	stat, err := self.FileReader.Stat()
	return stat, err
}

func getFSPathSpec(filename string) api.FSPathSpec {
	return paths.FSPathSpecFromClientPath(strings.TrimPrefix(filename, "fs:"))
}

func getDSPathSpec(filename string) api.DSPathSpec {
	return paths.DSPathSpecFromClientPath(strings.TrimPrefix(filename, "ds:"))
}
