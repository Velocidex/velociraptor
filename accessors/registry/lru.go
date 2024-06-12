//go:build windows
// +build windows

package registry

import (
	"time"

	"www.velocidex.com/golang/velociraptor/accessors"
)

type readDirLRUItem struct {
	children []accessors.FileInfo
	err      error
	age      time.Time
}
