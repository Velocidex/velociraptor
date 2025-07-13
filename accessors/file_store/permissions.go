package file_store

import (
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/paths"
)

var (
	ForbiddenPrefixes = []api.FSPathSpec{
		paths.CONFIG_ROOT.AsFilestorePath(),
		paths.USERS_ROOT.AsFilestorePath(),
		paths.ORGS_ROOT.AsFilestorePath(),
		paths.BACKUPS_ROOT,
	}
)

// Some parts of the filestore are blocked off from reading. This
// helps prevent circumvention of the ACL system by reading files
// directly from disk.
func isFileAccessible(filename api.FSPathSpec) error {
	for _, parent := range ForbiddenPrefixes {
		if path_specs.IsSubPath(parent, filename) {
			return acls.PermissionDenied
		}
	}
	return nil
}
