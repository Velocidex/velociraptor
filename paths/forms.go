package paths

import (
	"net/url"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

type FormUploadPathManager struct {
	filename string
	hash     string

	config_obj *config_proto.Config
}

func (self FormUploadPathManager) Path() api.FSPathSpec {
	return PUBLIC_ROOT.AddChild("temp", self.hash, self.filename)
}

// Calculate the URL by which the upload will be exported.
func (self FormUploadPathManager) URL() string {
	if self.config_obj.Client == nil || len(self.config_obj.Client.ServerUrls) == 0 {
		return ""
	}

	dest_url, err := url.Parse(self.config_obj.Client.ServerUrls[0])
	if err != nil {
		return ""
	}

	dest_url.Path = self.Path().AsClientPath()
	return dest_url.String()
}

func NewFormUploadPathManager(
	config_obj *config_proto.Config, filename string) *FormUploadPathManager {
	return &FormUploadPathManager{
		filename:   filename,
		hash:       ObfuscateName(config_obj, filename),
		config_obj: config_obj,
	}
}
