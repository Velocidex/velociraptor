package glob

import (
	"os"
	"time"
)

// Virtual FileInfo for root directory - represent all drives as
// directories.
type VirtualDirectoryPath struct {
	drive string
	// This holds information about the drive.
	data interface{}
}

func NewVirtualDirectoryPath(path string, data interface{}) *VirtualDirectoryPath {
	return &VirtualDirectoryPath{drive: path, data: data}
}

func (self *VirtualDirectoryPath) Name() string {
	return self.drive
}

func (self *VirtualDirectoryPath) Data() interface{} {
	return self.data
}

func (self *VirtualDirectoryPath) Size() int64 {
	return 0
}

func (self *VirtualDirectoryPath) Mode() os.FileMode {
	return os.ModeDir
}

func (self *VirtualDirectoryPath) ModTime() time.Time {
	return time.Now()
}

func (self *VirtualDirectoryPath) IsDir() bool {
	return true
}

func (self *VirtualDirectoryPath) Sys() interface{} {
	return nil
}

func (self *VirtualDirectoryPath) FullPath() string {
	return self.drive
}

func (self *VirtualDirectoryPath) Atime() TimeVal {
	return TimeVal{}
}

func (self *VirtualDirectoryPath) Mtime() TimeVal {
	return TimeVal{}
}

func (self *VirtualDirectoryPath) Ctime() TimeVal {
	return TimeVal{}
}
