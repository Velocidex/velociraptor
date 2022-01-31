package file

import (
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/json"
)

func init() {
	json.RegisterCustomEncoder(&OSFileInfo{}, accessors.MarshalGlobFileInfo)
}
