package paths

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"

	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/services"
)

func ObfuscateName(
	config_obj *config_proto.Config, name string) string {
	sha_sum := sha256.New()

	// Each org may store different files under the same name - we mix
	// in orgs id to keep them separated.
	_, err := sha_sum.Write([]byte(config_obj.ObfuscationNonce + name + config_obj.OrgId))
	if err != nil {
		return name
	}

	return hex.EncodeToString(sha_sum.Sum(nil))

}

type InventoryPathManager struct {
	org_config_obj *config_proto.Config
	root           api.FSPathSpec
}

// NOTE: The InventoryPathManager must be used with the root org's
// filestore, even though it is instantiated with the org's
// config. This is because inventory files are **always** written to
// the root's public/ directory so they can be exported through the
// web server.

// In order to enforce this, the prototype of this function is
// different than usual and returns the root filestore as well.
func (self InventoryPathManager) Path() (api.FSPathSpec, api.FileStore, error) {
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

	return self.root, file_store_factory, nil
}

func NewInventoryPathManager(config_obj *config_proto.Config,
	tool *artifacts_proto.Tool) *InventoryPathManager {
	if tool.FilestorePath == "" {
		tool.FilestorePath = ObfuscateName(config_obj, tool.Name)
	}

	return &InventoryPathManager{
		org_config_obj: config_obj,
		root:           PUBLIC_ROOT.AddChild(tool.FilestorePath),
	}
}
