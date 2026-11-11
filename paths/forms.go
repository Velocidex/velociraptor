package paths

import (
	"net/url"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

type FormUploadPathManager struct {
	filename string
	hash     string

	org_config_obj *config_proto.Config
}

func (self FormUploadPathManager) Path() api.FSPathSpec {
	return PUBLIC_ROOT.AddChild("temp", self.hash, self.filename)
}

// Calculate the URL by which the upload will be exported. The URL
// must contain the nonce for the relevant org so the client can fetch
// it with
func (self FormUploadPathManager) URL() string {
	if self.org_config_obj.Client == nil ||
		self.org_config_obj.Client.Nonce == "" ||
		len(self.org_config_obj.Client.ServerUrls) == 0 ||
		self.org_config_obj.Frontend == nil {
		return ""
	}

	dest_url, err := url.Parse(self.org_config_obj.Client.ServerUrls[0])
	if err != nil {
		return ""
	}

	// Incorporate the org nonce into the URL.
	path := PUBLIC_ROOT.AddChild(self.org_config_obj.Client.Nonce,
		"temp", self.hash, self.filename).AsClientPath()
	dest_url.Path = self.org_config_obj.Frontend.BasePath + path

	// Force https scheme instead of wss.
	if dest_url.Scheme == "wss" {
		dest_url.Scheme = "https"
	}

	return dest_url.String()
}

func NewFormUploadPathManager(
	config_obj *config_proto.Config, filename string) *FormUploadPathManager {

	// The hash is built by using a combination of filename and org
	// name to make it globally unique.
	return &FormUploadPathManager{
		filename:       filename,
		hash:           ObfuscateName(config_obj, filename),
		org_config_obj: config_obj,
	}
}
