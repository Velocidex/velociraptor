package glob

import (
	"os"
	"time"

	"github.com/Velocidex/json"
	"www.velocidex.com/golang/velociraptor/utils"
)

func MarshalGlobFileInfo(v interface{}, opts *json.EncOpts) ([]byte, error) {
	self, ok := v.(FileInfo)
	if !ok {
		return nil, json.EncoderCallbackSkip
	}

	return json.MarshalWithOptions(&struct {
		FullPath string
		Size     int64
		Mode     os.FileMode
		ModeStr  string
		ModTime  time.Time
		Sys      interface{}
		Mtime    utils.TimeVal
		Ctime    utils.TimeVal
		Atime    utils.TimeVal
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
	}, opts)
}
