package file_store

// This implements a filesystem accessor which can be used to access
// the filestore. This allows us to run globs on the file store
// regardless of the specific filestore implementation.

import (
	"context"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/glob"
)

func (self *FileStoreFileInfo) Data() interface{} {
	return self._data
}

func (self *FileStoreFileInfo) FullPath() string {
	return self._full_path
}

func (self *FileStoreFileInfo) Mtime() glob.TimeVal {
	return glob.TimeVal{}
}

func (self *FileStoreFileInfo) Ctime() glob.TimeVal {
	return glob.TimeVal{}
}

func (self *FileStoreFileInfo) Atime() glob.TimeVal {
	return glob.TimeVal{}
}

func (self *FileStoreFileInfo) IsLink() bool {
	return self.Mode()&os.ModeSymlink != 0
}

func (self *FileStoreFileInfo) GetLink() (string, error) {
	target, err := os.Readlink(self._full_path)
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
		Mtime    glob.TimeVal
		Ctime    glob.TimeVal
		Atime    glob.TimeVal
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

// Real implementation for non windows OSs:
type FileStoreFileSystemAccessor struct {
	file_store FileStore
}

func (self FileStoreFileSystemAccessor) New(ctx context.Context) glob.FileSystemAccessor {
	return &FileStoreFileSystemAccessor{self.file_store}
}

func (self FileStoreFileSystemAccessor) Lstat(filename string) (glob.FileInfo, error) {
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

	return file, nil
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

func GetFileStoreFileSystemAccessor(
	config_obj *config_proto.Config) *FileStoreFileSystemAccessor {
	return &FileStoreFileSystemAccessor{&DirectoryFileStore{config_obj}}
}
