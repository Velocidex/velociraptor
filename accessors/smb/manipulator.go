package smb

import (
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/utils"
)

type SMBPathManipulator struct {
	accessors.GenericPathManipulator
}

func (self SMBPathManipulator) PathJoin(path *accessors.OSPath) string {
	result := self.AsPathSpec(path)

	if result.GetDelegateAccessor() == "" && result.GetDelegatePath() == "" {
		return result.Path
	}
	return result.String()
}

func (self SMBPathManipulator) AsPathSpec(path *accessors.OSPath) *accessors.PathSpec {
	result := &accessors.PathSpec{}

	// First component must lead with a \\
	if len(path.Components) > 0 {
		result.Path = "\\" + utils.JoinComponents(path.Components, "\\")
	} else {
		result.Path = ""
	}
	return result
}
