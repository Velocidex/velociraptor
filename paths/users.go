package paths

import "www.velocidex.com/golang/velociraptor/constants"

type UserPathManager struct {
	Name string
}

func (self UserPathManager) Path() string {
	return constants.USER_URN + self.Name
}

func (self UserPathManager) Directory() string {
	return constants.USER_URN
}

func (self UserPathManager) GUIOptions() string {
	return constants.USER_URN + "/gui/" + self.Name
}
