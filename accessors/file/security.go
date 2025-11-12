package file

import (
	"sync"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	mu sync.Mutex

	allowedPrefixes *utils.PrefixTree
	deniedPrefixes  *utils.PrefixTree
	DeniedError     = utils.Wrap(acls.PermissionDenied, "No accesss to filesystem path")
)

func SetPrefixes(allowed *utils.PrefixTree, denied *utils.PrefixTree) {
	mu.Lock()
	defer mu.Unlock()

	allowedPrefixes = allowed
	deniedPrefixes = denied
}

func CheckPath(full_path string) error {
	destination_path, err := accessors.NewNativePath(full_path)
	if err != nil || len(destination_path.Components) == 0 {
		return err
	}

	return CheckPrefix(destination_path)
}

func CheckPrefix(full_path *accessors.OSPath) error {
	mu.Lock()
	defer mu.Unlock()

	return CheckAccessForPrefixes(full_path.Components, allowedPrefixes, deniedPrefixes)
}

func CheckAccessForPrefixes(components []string,
	allowed *utils.PrefixTree,
	denied *utils.PrefixTree) error {

	// Check denies first
	if denied != nil {
		match, denied_depth := denied.Present(components)
		if match {
			// If there is a more specific allow rule, then allow it,
			// otherwise we deny it.
			if allowed != nil {
				match, allowed_depth := allowed.Present(components)

				// If the allowed prefix is longer than the denied prefix,
				// then allow it.
				if match && allowed_depth > denied_depth {
					return nil
				}
			}
			return DeniedError
		}
	}

	// All files are allowed
	if allowed == nil {
		return nil
	}

	if len(components) == 0 {
		return nil
	}

	// There is only an AllowedPrefixes and no deny prefix, this means
	// we deny anything not inside the AllowedPrefixes.
	match, _ := allowed.Present(components)
	if match {
		return nil
	}

	return DeniedError
}
