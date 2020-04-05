package glob

import (
	"os"

	"www.velocidex.com/golang/velociraptor/utils"
)

type FileInfoAdapter struct {
	os.FileInfo
}

func (self FileInfoAdapter) ModTime() utils.Time {
	return utils.Time{self.FileInfo.ModTime()}
}
