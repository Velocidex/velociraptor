//go:build darwin && !cgo
// +build darwin,!cgo

package process

import (
	"errors"

	"www.velocidex.com/golang/velociraptor/accessors"
)

var (
	notSupportedError = errors.New("ProcessAccessor: This binary is not build with cgo support. Process access not enabled.")
)

func (self *ProcessAccessor) OpenWithOSPath(
	path *accessors.OSPath) (accessors.ReadSeekCloser, error) {
	return nil, notSupportedError
}
