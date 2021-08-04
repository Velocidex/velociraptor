package paths

import "www.velocidex.com/golang/velociraptor/file_store/api"

type ServerStatePathManager struct{}

func (self *ServerStatePathManager) Path() api.DSPathSpec {
	return CONFIG_ROOT.AddChild("server_state")
}
