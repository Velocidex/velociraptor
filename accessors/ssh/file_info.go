package ssh

import (
	"errors"
	"os"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/pkg/sftp"
	"www.velocidex.com/golang/velociraptor/accessors"
)

type SFTPFileInfo struct {
	_FileInfo  os.FileInfo
	_full_path *accessors.OSPath
}

func NewSFTPFileInfo(base os.FileInfo, path *accessors.OSPath) *SFTPFileInfo {
	return &SFTPFileInfo{
		_FileInfo:  base,
		_full_path: path,
	}
}

func (self *SFTPFileInfo) OSPath() *accessors.OSPath {
	return self._full_path
}

func (self *SFTPFileInfo) Size() int64 {
	return self._FileInfo.Size()
}

func (self *SFTPFileInfo) Name() string {
	return self._FileInfo.Name()
}

func (self *SFTPFileInfo) IsDir() bool {
	return self._FileInfo.IsDir()
}

func (self *SFTPFileInfo) ModTime() time.Time {
	return self._FileInfo.ModTime()
}

func (self *SFTPFileInfo) Mode() os.FileMode {
	return self._FileInfo.Mode()
}

func (self *SFTPFileInfo) Sys() interface{} {
	return self._FileInfo.Sys()
}

func (self *SFTPFileInfo) Dev() uint64 {
	return 0
}

func (self *SFTPFileInfo) Data() *ordereddict.Dict {
	result := ordereddict.NewDict()
	return result
}

func (self *SFTPFileInfo) FullPath() string {
	return self._full_path.String()
}

func (self *SFTPFileInfo) IsLink() bool {
	return self.Mode()&os.ModeSymlink != 0
}

func (self *SFTPFileInfo) GetLink() (*accessors.OSPath, error) {
	return nil, errors.New("Symlink not supported")
}

func (self *SFTPFileInfo) _Sys() *sftp.FileStat {
	return self._FileInfo.Sys().(*sftp.FileStat)
}

func (self *SFTPFileInfo) Btime() time.Time {
	return time.Time{}
}

func (self *SFTPFileInfo) Mtime() time.Time {
	return time.Unix(int64(self._Sys().Mtime), 0)
}

func (self *SFTPFileInfo) Ctime() time.Time {
	return time.Time{}
}

func (self *SFTPFileInfo) Atime() time.Time {
	return time.Unix(int64(self._Sys().Atime), 0)
}

type SFTPFileWrapper struct {
	*sftp.File
}
