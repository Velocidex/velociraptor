package vtesting

import (
	"os"
	"time"
)

type MockFileInfo struct {
	Name_     string
	FullPath_ string
	Size_     int64
}

func (self MockFileInfo) Data() interface{}        { return nil }
func (self MockFileInfo) Name() string             { return self.Name_ }
func (self MockFileInfo) Size() int64              { return self.Size_ }
func (self MockFileInfo) Mode() os.FileMode        { return os.ModePerm }
func (self MockFileInfo) ModTime() time.Time       { return time.Time{} }
func (self MockFileInfo) IsDir() bool              { return true }
func (self MockFileInfo) Sys() interface{}         { return nil }
func (self MockFileInfo) FullPath() string         { return self.FullPath_ }
func (self MockFileInfo) Btime() time.Time         { return time.Time{} }
func (self MockFileInfo) Mtime() time.Time         { return time.Time{} }
func (self MockFileInfo) Atime() time.Time         { return time.Time{} }
func (self MockFileInfo) Ctime() time.Time         { return time.Time{} }
func (self MockFileInfo) IsLink() bool             { return false }
func (self MockFileInfo) GetLink() (string, error) { return "", nil }
