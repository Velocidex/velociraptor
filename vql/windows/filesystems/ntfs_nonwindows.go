//go:build !windows
// +build !windows

package filesystems

import (
	"errors"

	"www.velocidex.com/golang/velociraptor/glob"
)

func discoverVSS() ([]glob.FileInfo, error) {
	return nil, errors.New("Not supported on non-Windows OS")
}

func discoverLogicalDisks() ([]glob.FileInfo, error) {
	return nil, errors.New("Not supported on non-Windows OS")
}
