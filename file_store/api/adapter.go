package api

import (
	"os"

	"github.com/Velocidex/ordereddict"
	"github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/glob"
)

// Implement the glob.FileInfo
type FileInfoAdapter struct {
	os.FileInfo

	full_path string
	_data     interface{}
}

func NewFileInfoAdapter(fd os.FileInfo, full_path string, data interface{}) *FileInfoAdapter {
	return &FileInfoAdapter{
		FileInfo:  fd,
		full_path: full_path,
		_data:     data,
	}
}

func (self FileInfoAdapter) Data() interface{} {
	if self._data == nil {
		return ordereddict.NewDict()
	}

	return self._data
}

func (self FileInfoAdapter) FullPath() string {
	return self.full_path
}

func (self FileInfoAdapter) Mtime() glob.TimeVal {
	return glob.TimeVal{}
}

func (self FileInfoAdapter) Atime() glob.TimeVal {
	return glob.TimeVal{}
}

func (self FileInfoAdapter) Ctime() glob.TimeVal {
	return glob.TimeVal{}
}

func (self FileInfoAdapter) IsLink() bool {
	return self.Mode()&os.ModeSymlink != 0
}

func (self FileInfoAdapter) GetLink() (string, error) {
	return "", errors.New("Not implemented")
}

type FileAdapter struct {
	*os.File

	FullPath string
}

func (self *FileAdapter) Stat() (glob.FileInfo, error) {
	stat, err := self.File.Stat()
	if err != nil {
		return nil, err
	}
	return NewFileInfoAdapter(stat, self.FullPath, nil), nil
}

type FileReaderAdapter struct {
	FileReader
}

func (self *FileReaderAdapter) Stat() (os.FileInfo, error) {
	stat, err := self.FileReader.Stat()
	return stat, err
}
