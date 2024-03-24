package raw_registry

import "www.velocidex.com/golang/velociraptor/accessors"

type readDirLRUItem struct {
	children []accessors.FileInfo
	err      error
}
