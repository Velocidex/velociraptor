package paths

import (
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

type FormUploadPathManager struct {
	filename string
	hash     string
}

func (self FormUploadPathManager) Path() api.FSPathSpec {
	return PUBLIC_ROOT.AddChild("temp", self.hash, self.filename)
}

func NewFormUploadPathManager(
	config_obj *config_proto.Config, filename string) *FormUploadPathManager {

	// The hash is built by using a combination of filename and org
	// name to make it globally unique.
	return &FormUploadPathManager{
		filename: filename,
		hash:     ObfuscateName(config_obj, filename),
	}
}
