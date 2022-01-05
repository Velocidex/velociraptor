//go:build !windows
// +build !windows

package filesystems

import (
	"errors"

	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/vql"
)

func discoverVSS() ([]glob.FileInfo, error) {
	return nil, errors.New("Not supported on non-Windows OS")
}

func discoverLogicalDisks() ([]glob.FileInfo, error) {
	return nil, errors.New("Not supported on non-Windows OS")
}

func (self *NTFSFileSystemAccessor) GetRoot(path string) (
	device string, subpath string, err error) {

	config, ok := vql.GetServerConfig(self.scope)
	if !ok {
		return "/", path, errors.New("cannot determine the NTFS device")
	}

	return config.Device, path, nil
}
