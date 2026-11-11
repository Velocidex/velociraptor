package paths

import (
	"errors"
	"net/url"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
)

type FormUploadPathManager struct {
	filename string
	hash     string

	org_config_obj *config_proto.Config
}

// Form uploads are written to the org's own filestore.
func (self FormUploadPathManager) Path() (api.FSPathSpec, api.FileStore, error) {
	file_store_factory := file_store.GetFileStore(self.org_config_obj)
	if file_store_factory == nil {
		return nil, nil, errors.New("No filestore configured")
	}

	return self.path(), file_store_factory, nil
}

func (self FormUploadPathManager) path() api.FSPathSpec {
	return PUBLIC_ROOT.AddChild("temp", self.hash, self.filename)
}

// Calculate the URL by which the upload will be exported. Uploads
// outside the root org carry the org's client nonce as the second
// path component so the public handler reads them from that org's
// filestore without exposing the org id.
func (self FormUploadPathManager) URL() string {
	if self.org_config_obj.Client == nil || len(self.org_config_obj.Client.ServerUrls) == 0 {
		return ""
	}

	dest_url, err := url.Parse(self.org_config_obj.Client.ServerUrls[0])
	if err != nil {
		return ""
	}

	path := self.path()
	if !utils.IsRootOrg(self.org_config_obj.OrgId) &&
		self.org_config_obj.Client.Nonce != "" {
		path = PUBLIC_ROOT.AddChild(self.org_config_obj.Client.Nonce,
			"temp", self.hash, self.filename)
	}

	// Force https scheme instead of wss.
	dest_url.Path = path.AsClientPath()
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
