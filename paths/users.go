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
