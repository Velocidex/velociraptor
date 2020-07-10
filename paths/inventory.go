package paths

import (
	"crypto/sha256"
	"encoding/hex"
	"path"

	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func ObfuscateName(
	config_obj *config_proto.Config, name string) string {
	sha_sum := sha256.New()
	sha_sum.Write([]byte(config_obj.ObfuscationNonce + name))

	return hex.EncodeToString(sha_sum.Sum(nil))

}

func NewInventoryPathManager(
	config_obj *config_proto.Config, tool *artifacts_proto.Tool) *ClientPathManager {
	if tool.FilestorePath == "" {
		tool.FilestorePath = ObfuscateName(config_obj, tool.Name)
	}

	return &ClientPathManager{
		path: path.Join("/public/", tool.FilestorePath),
	}
}
