package glob

import (
	"os"
	"time"

	"github.com/Velocidex/json"
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
		Mtime    time.Time
		Ctime    time.Time
		Atime    time.Time
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
