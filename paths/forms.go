package paths

import (
	"errors"
	"net/url"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/services"
)

type FormUploadPathManager struct {
	filename string
	hash     string

	org_config_obj *config_proto.Config
}

// NOTE: The FormUploadPathManager must be used with the root org's
// filestore, even though it is instantiated with the org's
// config. This is because form upload files are **always** written to
// the root's public/ directory so they can be exported through the
// web server.

// In order to enforce this, the prototype of this function is
// different than usual and returns the root filestore as well.
func (self FormUploadPathManager) Path() (api.FSPathSpec, api.FileStore, error) {
	// All tools are stored at the global public directory which is
	// mapped to a http static handler. The downloaded URL is
	// regardless of org - however each org has a different download
	// name. We need to write the tool on the root org's public
	// directory.
	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil, nil, err
	}

	root_org_config, err := org_manager.GetOrgConfig(services.ROOT_ORG_ID)
	if err != nil {
		return nil, nil, err
	}

	file_store_factory := file_store.GetFileStore(root_org_config)
	if file_store_factory == nil {
		return nil, nil, errors.New("No filestore configured")
	}

	return self.path(), file_store_factory, nil
}

func (self FormUploadPathManager) path() api.FSPathSpec {
	return PUBLIC_ROOT.AddChild("temp", self.hash, self.filename)
}

// Calculate the URL by which the upload will be exported. Uploads are
// shared by all orgs in the same public directory but their hashes
// prevent collisions.
func (self FormUploadPathManager) URL() string {
	if self.org_config_obj.Client == nil || len(self.org_config_obj.Client.ServerUrls) == 0 {
		return ""
	}

	dest_url, err := url.Parse(self.org_config_obj.Client.ServerUrls[0])
	if err != nil {
		return ""
	}

	// Force https scheme instead of wss.
	dest_url.Path = self.path().AsClientPath()
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
