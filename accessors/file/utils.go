package file

import (
	"os"

	"www.velocidex.com/golang/velociraptor/acls"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

// Create the output file after checking that the query may write
// there.
func OpenFileForWriting(scope vfilter.Scope, path string,
	flags int, permissions os.FileMode) (*os.File, error) {
	err := vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_WRITE)
	if err != nil {
		return nil, err
	}

	path, err = CheckPath(path)
	if err != nil {
		return nil, err
	}

	return os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, permissions)
}

func CreateIntermediateDirectories(scope vfilter.Scope,
	path string, dir_permissions os.FileMode) error {
	err := vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_WRITE)
	if err != nil {
		return err
	}

	path, err = CheckPath(path)
	if err != nil {
		return err
	}

	return os.MkdirAll(path, dir_permissions)
}
