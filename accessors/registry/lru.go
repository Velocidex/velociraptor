//go:build windows
// +build windows

package registry

import "www.velocidex.com/golang/velociraptor/accessors"

type readDirLRUItem struct {
	children []accessors.FileInfo
	err      error
}
