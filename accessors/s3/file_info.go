package s3

import (
	"errors"
	"os"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
)

type S3FileInfo struct {
	path     *accessors.OSPath
	is_dir   bool
	size     int64
	mod_time time.Time
}

func (self *S3FileInfo) IsDir() bool {
	return self.is_dir
}

func (self *S3FileInfo) Size() int64 {
	return self.size
}

func (self *S3FileInfo) Data() *ordereddict.Dict {
	result := ordereddict.NewDict()
	return result
}

func (self *S3FileInfo) Name() string {
	return self.path.Basename()
}

func (self *S3FileInfo) Mode() os.FileMode {
	var result os.FileMode = 0755
	if self.IsDir() {
		result |= os.ModeDir
	}
	return result
}

func (self *S3FileInfo) ModTime() time.Time {
	return self.mod_time
}

func (self *S3FileInfo) FullPath() string {
	return self.path.String()
}

func (self *S3FileInfo) OSPath() *accessors.OSPath {
	return self.path.Copy()
}

func (self *S3FileInfo) Mtime() time.Time {
	return self.mod_time
}

func (self *S3FileInfo) Ctime() time.Time {
	return self.Mtime()
}

func (self *S3FileInfo) Btime() time.Time {
	return self.Mtime()
}

func (self *S3FileInfo) Atime() time.Time {
	return self.Mtime()
}

// Not supported
func (self *S3FileInfo) IsLink() bool {
	return false
}

func (self *S3FileInfo) GetLink() (*accessors.OSPath, error) {
	return nil, errors.New("Not implemented")
}
