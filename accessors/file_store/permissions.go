package file_store

import (
	"sync"

	"www.velocidex.com/golang/velociraptor/accessors/file"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	mu sync.Mutex

	// By default all filestore access is allowed.
	allowedPrefixes *utils.PrefixTree
	deniedPrefixes  *utils.PrefixTree

	DeniedError = utils.Wrap(acls.PermissionDenied, "No accesss to file store path")
)

func SetPrefixes(allowed *utils.PrefixTree, denied *utils.PrefixTree) {
	mu.Lock()
	defer mu.Unlock()

	allowedPrefixes = allowed
	deniedPrefixes = denied
}

// Some parts of the filestore are blocked off from reading. This
// helps prevent circumvention of the ACL system by reading files
// directly from disk.
func IsFileAccessible(filename api.FSPathSpec) error {
	mu.Lock()
	defer mu.Unlock()

	return file.CheckAccessForPrefixes(
		filename.Components(), allowedPrefixes, deniedPrefixes)
}
