package file_store_test

import (
	"testing"

	"www.velocidex.com/golang/velociraptor/accessors/file_store"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services/sanity"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func TestFSAccessorSecurity(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	sanity_service := &sanity.SanityChecks{}

	// No security set - everything is allowed.
	config_obj.Security = &config_proto.Security{
		DeniedFsAccessorPrefix: []string{
			"XXXXX",
		},
	}

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

	// Make sure we treat files with empty components correcrly.
	filename1 := path_specs.NewUnsafeFilestorePath("backups", "file")
	assert.Error(t, file_store.IsFileAccessible(filename1))

	filename2 := path_specs.NewUnsafeFilestorePath("", "backups", "file")
	assert.Error(t, file_store.IsFileAccessible(filename2))

	// Both pathspecs actually end up in the same filepath
	db, err := datastore.GetDB(config_obj)
	assert.NoError(t, err)
	assert.Equal(t,
		datastore.AsFilestoreFilename(db, config_obj, filename1),
		datastore.AsFilestoreFilename(db, config_obj, filename2))

	// Check permission filtering within the DeniedFsAccessorPrefix
	assert.Error(t, file_store.IsFileAccessible(
		paths.BACKUPS_ROOT.AddChild("File")))
	assert.NoError(t, file_store.IsFileAccessible(
		paths.PUBLIC_ROOT.AddChild("C.123")))
	assert.NoError(t, file_store.IsFileAccessible(
		paths.DOWNLOADS_ROOT.AddChild("C.123", "somefile.zip")))
}
