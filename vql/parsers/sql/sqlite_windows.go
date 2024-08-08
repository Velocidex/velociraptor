//go:build windows && amd64 && cgo
// +build windows,amd64,cgo

package sql

import (
	_ "www.velocidex.com/golang/velociraptor/vql/windows/filesystems"
)
