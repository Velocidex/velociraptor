package accessors

import (
	"os"
	"time"

	"www.velocidex.com/golang/velociraptor/json"
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
		Data     interface{}
		Mtime    time.Time
		Ctime    time.Time
		Atime    time.Time
	}{
		FullPath: self.FullPath(),
		Size:     self.Size(),
		Mode:     self.Mode(),
		ModeStr:  self.Mode().String(),
		ModTime:  self.ModTime(),
		Mtime:    self.Mtime(),
		Ctime:    self.Ctime(),
		Atime:    self.Atime(),
		Data:     self.Data(),
	}, opts)
}
