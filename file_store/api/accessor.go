package api

// This implements a filesystem accessor which can be used to access
// the generic filestore. This allows us to run globs on the file
// store regardless of the specific filestore implementation.  This
// accessor is for DirectoryFileStore

import (
	"os"
	"path"
	"regexp"
	"time"

	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/vfilter"
)

type FileStoreFileSystemAccessor struct {
	file_store FileStore
	config_obj *config_proto.Config
}

func NewFileStoreFileSystemAccessor(
	config_obj *config_proto.Config, fs FileStore) *FileStoreFileSystemAccessor {
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

	fullpath := NewUnsafeDatastorePath(
		utils.SplitComponents(filename)...).AsSafe()
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

func (self FileStoreFileSystemAccessor) ReadDir(path string) ([]glob.FileInfo, error) {
	fullpath := NewUnsafeDatastorePath(utils.SplitComponents(path)...).AsSafe()
	files, err := self.file_store.ListDirectory(fullpath)
	if err != nil {
		return nil, err
	}

	var result []glob.FileInfo
	for _, f := range files {
		result = append(result,
			&FileStoreFileInfo{
				FileInfo:   f,
				fullpath:   fullpath,
				config_obj: self.config_obj,
			})
	}

	return result, nil
}

func (self FileStoreFileSystemAccessor) Open(path string) (glob.ReadSeekCloser, error) {
	components := NewSafeDatastorePath(utils.SplitComponents(path)...)
	file, err := self.file_store.ReadFile(components)
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
	fullpath PathSpec,
	info os.FileInfo) *FileStoreFileInfo {
	return &FileStoreFileInfo{
		config_obj: config_obj,
		FileInfo:   info,
		fullpath:   fullpath,
	}
}

type FileStoreFileInfo struct {
	os.FileInfo
	fullpath   PathSpec
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

func (self *FileStoreFileInfo) FullPath() string {
	return self.fullpath.AsClientPath()
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
	FileReader
}

func (self *FileReaderAdapter) Stat() (os.FileInfo, error) {
	stat, err := self.FileReader.Stat()
	return stat, err
}
