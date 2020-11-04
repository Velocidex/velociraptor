package vtesting

import (
	"os"
	"time"

	"www.velocidex.com/golang/velociraptor/utils"
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
func (self MockFileInfo) Mtime() utils.TimeVal     { return utils.TimeVal{} }
func (self MockFileInfo) Atime() utils.TimeVal     { return utils.TimeVal{} }
func (self MockFileInfo) Ctime() utils.TimeVal     { return utils.TimeVal{} }
func (self MockFileInfo) IsLink() bool             { return false }
func (self MockFileInfo) GetLink() (string, error) { return "", nil }
