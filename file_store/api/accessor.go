package api

// This implements a filesystem accessor which can be used to access
// the generic filestore. This allows us to run globs on the file
// store regardless of the specific filestore implementation.  This
// accessor is for DirectoryFileStore

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"time"

	"www.velocidex.com/golang/velociraptor/json"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type FileStoreFileSystemAccessor struct {
	file_store FileStore
}

func NewFileStoreFileSystemAccessor(
	config_obj *config_proto.Config, fs FileStore) *FileStoreFileSystemAccessor {
	return &FileStoreFileSystemAccessor{fs}
}

func (self FileStoreFileSystemAccessor) New(
	scope vfilter.Scope) glob.FileSystemAccessor {
	return &FileStoreFileSystemAccessor{self.file_store}
}

func (self FileStoreFileSystemAccessor) Lstat(
	filename string) (glob.FileInfo, error) {
	lstat, err := self.file_store.StatFile(filename)
	if err != nil {
		return nil, err
	}

	return &FileStoreFileInfo{lstat, filename, nil}, nil
}

func (self FileStoreFileSystemAccessor) ReadDir(path string) ([]glob.FileInfo, error) {
	files, err := self.file_store.ListDirectory(path)
	if err != nil {
		return nil, err
	}

	var result []glob.FileInfo
	for _, f := range files {
		result = append(result,
			&FileStoreFileInfo{f, filepath.Join(path, f.Name()), nil})
	}

	return result, nil
}

func (self FileStoreFileSystemAccessor) Open(path string) (glob.ReadSeekCloser, error) {
	file, err := self.file_store.ReadFile(path)
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

type FileStoreFileInfo struct {
	os.FileInfo
	FullPath_ string
	Data_     interface{}
}

func (self FileStoreFileInfo) Name() string {
	return self.FileInfo.Name()
}

func (self *FileStoreFileInfo) Data() interface{} {
	if self.Data_ == nil {
		return ordereddict.NewDict()
	}

	return self.Data_
}

func (self *FileStoreFileInfo) FullPath() string {
	return self.FullPath_
}

func (self *FileStoreFileInfo) Mtime() utils.TimeVal {
	return utils.TimeVal{}
}

func (self *FileStoreFileInfo) Ctime() utils.TimeVal {
	return utils.TimeVal{}
}

func (self *FileStoreFileInfo) Atime() utils.TimeVal {
	return utils.TimeVal{}
}

func (self *FileStoreFileInfo) IsLink() bool {
	return self.Mode()&os.ModeSymlink != 0
}

func (self *FileStoreFileInfo) GetLink() (string, error) {
	target, err := os.Readlink(self.FullPath_)
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
		Mtime    utils.TimeVal
		Ctime    utils.TimeVal
		Atime    utils.TimeVal
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
