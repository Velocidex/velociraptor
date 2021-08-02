package api

import (
	"os"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Implement the glob.FileInfo
type FileInfoAdapter struct {
	os.FileInfo

	full_path PathSpec
	_data     interface{}
}

func NewFileInfoAdapter(fd os.FileInfo, full_path PathSpec, data interface{}) *FileInfoAdapter {
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
	return utils.JoinComponents(self.full_path.Components(), "/")
}

func (self FileInfoAdapter) Btime() time.Time {
	return time.Time{}
}

func (self FileInfoAdapter) Mtime() time.Time {
	return time.Time{}
}

func (self FileInfoAdapter) Atime() time.Time {
	return time.Time{}
}

func (self FileInfoAdapter) Ctime() time.Time {
	return time.Time{}
}

func (self FileInfoAdapter) IsLink() bool {
	return self.Mode()&os.ModeSymlink != 0
}

func (self FileInfoAdapter) GetLink() (string, error) {
	return "", errors.New("Not implemented")
}

// A Wrapper around a regular file to present the glob.FileInfo
// interface
type FileAdapter struct {
	*os.File

	FullPath PathSpec
}

func (self *FileAdapter) Stat() (glob.FileInfo, error) {
	stat, err := self.File.Stat()
	if err != nil {
		return nil, err
	}
	return NewFileInfoAdapter(stat, self.FullPath, nil), nil
}
