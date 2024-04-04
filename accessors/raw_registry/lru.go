package raw_registry

import (
	"www.velocidex.com/golang/regparser"
	"www.velocidex.com/golang/velociraptor/accessors"
)

type readDirLRUItem struct {
	children []accessors.FileInfo
	err      error

	key *regparser.CM_KEY_NODE
}
