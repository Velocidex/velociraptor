package paths

type ThirdPartyPathManager struct {
	binary_name string
}

func (self *ThirdPartyPathManager) Path() string {
	return "/public/" + self.binary_name
}

func NewThirdPartyPathManager(binary_name string) *ThirdPartyPathManager {
	return &ThirdPartyPathManager{binary_name}
}
