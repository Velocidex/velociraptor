package file_store

import (
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	AllowedPrefixes = utils.NewPrefixTree()
	DeniedError     = utils.Wrap(acls.PermissionDenied, "No accesss to file store path")
)

// Some parts of the filestore are blocked off from reading. This
// helps prevent circumvention of the ACL system by reading files
// directly from disk.
func isFileAccessible(filename api.FSPathSpec) error {
	components := filename.Components()
	if len(components) == 0 {
		return nil
	}

	if AllowedPrefixes.Present(components) {
		return nil
	}

	return DeniedError
}
