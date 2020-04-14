package file_store

// This implements a filesystem accessor which can be used to access
// the filestore. This allows us to run globs on the file store
// regardless of the specific filestore implementation.
// This accessor is for DirectoryFileStore

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"time"

	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/vfilter"
)

type DirectoryFileStoreFileInfo struct {
	os.FileInfo
	_full_path string
	_data      interface{}
}

func (self DirectoryFileStoreFileInfo) Name() string {
	return datastore.UnsanitizeComponent(self.FileInfo.Name())
}

func (self *DirectoryFileStoreFileInfo) Data() interface{} {
	return self._data
}

func (self *DirectoryFileStoreFileInfo) FullPath() string {
	return self._full_path
}

func (self *DirectoryFileStoreFileInfo) Mtime() glob.TimeVal {
	return glob.TimeVal{}
}

func (self *DirectoryFileStoreFileInfo) Ctime() glob.TimeVal {
	return glob.TimeVal{}
}

func (self *DirectoryFileStoreFileInfo) Atime() glob.TimeVal {
	return glob.TimeVal{}
}

func (self *DirectoryFileStoreFileInfo) IsLink() bool {
	return self.Mode()&os.ModeSymlink != 0
}

func (self *DirectoryFileStoreFileInfo) GetLink() (string, error) {
	target, err := os.Readlink(self._full_path)
	if err != nil {
		return "", err
	}
	return target, nil
}

func (self *DirectoryFileStoreFileInfo) MarshalJSON() ([]byte, error) {
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

func (self *DirectoryFileStoreFileInfo) UnmarshalJSON(data []byte) error {
	return nil
}

type DirectoryFileStoreFileSystemAccessor struct {
	file_store *DirectoryFileStore
}

func (self DirectoryFileStoreFileSystemAccessor) New(
	scope *vfilter.Scope) glob.FileSystemAccessor {
	return &DirectoryFileStoreFileSystemAccessor{self.file_store}
}

func (self DirectoryFileStoreFileSystemAccessor) Lstat(
	filename string) (glob.FileInfo, error) {
	lstat, err := self.file_store.StatFile(filename)
	if err != nil {
		return nil, err
	}

	return &DirectoryFileStoreFileInfo{lstat, filename, nil}, nil
}

func (self DirectoryFileStoreFileSystemAccessor) ReadDir(path string) ([]glob.FileInfo, error) {
	files, err := self.file_store.ListDirectory(path)
	if err != nil {
		return nil, err
	}

	var result []glob.FileInfo
	for _, f := range files {
		result = append(result,
			&DirectoryFileStoreFileInfo{f, filepath.Join(path, f.Name()), nil})
	}

	return result, nil
}

func (self DirectoryFileStoreFileSystemAccessor) Open(path string) (glob.ReadSeekCloser, error) {
	file, err := self.file_store.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return &FileReaderAdapter{file}, nil
}

var DirectoryFileStoreFileSystemAccessor_re = regexp.MustCompile("/")

func (self DirectoryFileStoreFileSystemAccessor) PathSplit(path string) []string {
	return DirectoryFileStoreFileSystemAccessor_re.Split(path, -1)
}

func (self DirectoryFileStoreFileSystemAccessor) PathJoin(root, stem string) string {
	return path.Join(root, stem)
}

func (self *DirectoryFileStoreFileSystemAccessor) GetRoot(path string) (string, string, error) {
	return "/", path, nil
}
