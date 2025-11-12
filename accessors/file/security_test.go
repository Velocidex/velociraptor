package file_test

import (
	"testing"

	"www.velocidex.com/golang/velociraptor/accessors/file"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services/sanity"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func TestAccessorFileSecurity(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	sanity_service := &sanity.SanityChecks{}

	// No security set - everything is allowed.
	config_obj.Security = &config_proto.Security{}

	sanity_service.CheckSecuritySettings(config_obj)

	assert.NoError(t, file.CheckPath("/tmp/foo/bar/baz"))
	assert.NoError(t, file.CheckPath("/etc/passwd"))

	// Default server configuration:
	// 1. Block access to the entire filesystem
	// 2. Allow access to the /tmp/ directory
	config_obj.Security = &config_proto.Security{
		AllowedFileAccessorPrefix: []string{
			"/tmp",
		},
	}

	sanity_service.CheckSecuritySettings(config_obj)

	assert.NoError(t, file.CheckPath("/tmp/foo/bar/baz"))
	assert.Error(t, file.CheckPath("/etc/passwd"))

	// Allow access to the whole server but reject access to the
	// datastore.
	config_obj.Security = &config_proto.Security{
		DeniedFileAccessorPrefix: []string{
			"/opt/velociraptor",
		},
	}

	sanity_service.CheckSecuritySettings(config_obj)

	assert.Error(t, file.CheckPath("/opt/velociraptor/downloads/filename.zip"))
	assert.NoError(t, file.CheckPath("/etc/passwd"))

	// Allow file access to a small part of the filestore, as well as
	// the /tmp/
	config_obj.Security = &config_proto.Security{
		DeniedFileAccessorPrefix: []string{
			"/opt/velociraptor",
		},
		AllowedFileAccessorPrefix: []string{
			"/tmp",
			"/opt/velociraptor/downloads",
		},
	}

	sanity_service.CheckSecuritySettings(config_obj)

	// Backups are not allowed.
	assert.Error(t, file.CheckPath("/opt/velociraptor/backups/filename.zip"))

	// Downloads are specifically allowed.
	assert.NoError(t, file.CheckPath("/opt/velociraptor/downloads/filename.zip"))

	// Random locations on the server are not allowed.
	assert.Error(t, file.CheckPath("/etc/passwd"))

	// Locations in /tmp are allowed.
	assert.NoError(t, file.CheckPath("/tmp"))

	// Allow to read everywhere on the server, except for the file store. But also allow reading in the downloads folder.
	config_obj.Security = &config_proto.Security{
		DeniedFileAccessorPrefix: []string{
			"/opt/velociraptor",
		},
		AllowedFileAccessorPrefix: []string{
			"/",
			"/opt/velociraptor/downloads",
		},
	}

	sanity_service.CheckSecuritySettings(config_obj)

	// Backups are not allowed since they are in the file store.
	assert.Error(t, file.CheckPath("/opt/velociraptor/backups/filename.zip"))

	// Downloads are specifically allowed.
	assert.NoError(t, file.CheckPath("/opt/velociraptor/downloads/filename.zip"))

	// Random locations on the server are allowed.
	assert.NoError(t, file.CheckPath("/etc/passwd"))

	// Locations in /tmp are allowed.
	assert.NoError(t, file.CheckPath("/tmp"))

	// Deny access to the filesystem but allow access to /tmp/
	config_obj.Security = &config_proto.Security{
		AllowedFileAccessorPrefix: []string{
			"/tmp",
		},
	}

	sanity_service.CheckSecuritySettings(config_obj)

	assert.Error(t, file.CheckPath("/opt/velociraptor/downloads/filename.zip"))
	assert.NoError(t, file.CheckPath("/tmp/some/long/path.txt"))
}
