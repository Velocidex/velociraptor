package file_test

import (
	"strings"
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

	_, err := file.CheckPath("/tmp/foo/bar/baz")
	assert.NoError(t, err)

	// Check that path traversal chars are removed.
	path, err := file.CheckPath("/tmp/../../../../bar/baz")
	assert.NoError(t, err)
	assert.True(t, !strings.Contains(path, ".."))

	_, err = file.CheckPath("/etc/passwd")
	assert.NoError(t, err)

	// Default server configuration:
	// 1. Block access to the entire filesystem
	// 2. Allow access to the /tmp/ directory
	config_obj.Security = &config_proto.Security{
		AllowedFileAccessorPrefix: []string{
			"/tmp",
		},
	}

	sanity_service.CheckSecuritySettings(config_obj)

	_, err = file.CheckPath("/tmp/foo/bar/baz")
	assert.NoError(t, err)

	_, err = file.CheckPath("/etc/passwd")
	assert.Error(t, err)

	// Allow access to the whole server but reject access to the
	// datastore.
	config_obj.Security = &config_proto.Security{
		DeniedFileAccessorPrefix: []string{
			"/opt/velociraptor",
		},
	}

	sanity_service.CheckSecuritySettings(config_obj)

	_, err = file.CheckPath("/opt/velociraptor/downloads/filename.zip")
	assert.Error(t, err)

	_, err = file.CheckPath("/etc/passwd")
	assert.NoError(t, err)

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
	_, err = file.CheckPath("/opt/velociraptor/backups/filename.zip")
	assert.Error(t, err)

	// Downloads are specifically allowed.
	_, err = file.CheckPath("/opt/velociraptor/downloads/filename.zip")
	assert.NoError(t, err)

	// Random locations on the server are not allowed.
	_, err = file.CheckPath("/etc/passwd")
	assert.Error(t, err)

	// Locations in /tmp are allowed.
	_, err = file.CheckPath("/tmp")
	assert.NoError(t, err)

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
	_, err = file.CheckPath("/opt/velociraptor/backups/filename.zip")
	assert.Error(t, err)

	// Downloads are specifically allowed.
	_, err = file.CheckPath("/opt/velociraptor/downloads/filename.zip")
	assert.NoError(t, err)

	// Random locations on the server are allowed.
	_, err = file.CheckPath("/etc/passwd")
	assert.NoError(t, err)

	// Locations in /tmp are allowed.
	_, err = file.CheckPath("/tmp")
	assert.NoError(t, err)

	// Deny access to the filesystem but allow access to /tmp/
	config_obj.Security = &config_proto.Security{
		AllowedFileAccessorPrefix: []string{
			"/tmp",
		},
	}

	sanity_service.CheckSecuritySettings(config_obj)

	_, err = file.CheckPath("/opt/velociraptor/downloads/filename.zip")
	assert.Error(t, err)

	_, err = file.CheckPath("/tmp/some/long/path.txt")
	assert.NoError(t, err)
}
