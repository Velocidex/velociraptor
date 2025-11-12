package file_store_test

import (
	"testing"

	"www.velocidex.com/golang/velociraptor/accessors/file_store"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services/sanity"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func TestFSAccessorSecurity(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	sanity_service := &sanity.SanityChecks{}

	// No security set - everything is allowed.
	config_obj.Security = &config_proto.Security{}

	sanity_service.CheckSecuritySettings(config_obj)

	assert.NoError(t, file_store.IsFileAccessible(paths.BACKUPS_ROOT.AddChild("File")))
	assert.NoError(t, file_store.IsFileAccessible(paths.PUBLIC_ROOT.AddChild("C.123")))

	// Block access to sensitive locations
	config_obj.Security = &config_proto.Security{
		DeniedFsAccessorPrefix: []string{
			"backups",
			"config",
		},
	}

	sanity_service.CheckSecuritySettings(config_obj)

	assert.Error(t, file_store.IsFileAccessible(paths.BACKUPS_ROOT.AddChild("File")))
	assert.NoError(t, file_store.IsFileAccessible(paths.PUBLIC_ROOT.AddChild("C.123")))
	assert.NoError(t, file_store.IsFileAccessible(paths.DOWNLOADS_ROOT.AddChild(
		"C.123", "somefile.zip")))

}
