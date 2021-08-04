package paths

import (
	"crypto/sha256"
	"encoding/hex"

	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

func ObfuscateName(
	config_obj *config_proto.Config, name string) string {
	sha_sum := sha256.New()
	_, err := sha_sum.Write([]byte(config_obj.ObfuscationNonce + name))
	if err != nil {
		return name
	}

	return hex.EncodeToString(sha_sum.Sum(nil))

}

type InventoryPathManager struct {
	root api.PathSpec
}

func (self InventoryPathManager) Path() api.PathSpec {
	return self.root
}

func NewInventoryPathManager(config_obj *config_proto.Config,
	tool *artifacts_proto.Tool) *InventoryPathManager {
	if tool.FilestorePath == "" {
		tool.FilestorePath = ObfuscateName(config_obj, tool.Name)
	}

	return &InventoryPathManager{
		root: PUBLIC_ROOT.AddChild(tool.FilestorePath),
	}
}
