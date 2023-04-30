package paths

import (
	"github.com/google/uuid"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

type TempPathManager struct {
	filename string
}

func (self TempPathManager) Path() api.FSPathSpec {
	return TEMP_ROOT.AddChild(self.filename)
}

func NewTempPathManager(filename string) *TempPathManager {
	if filename == "" {
		filename = uuid.New().String()
	}

	return &TempPathManager{filename: filename}
}
