package paths

import "www.velocidex.com/golang/velociraptor/file_store/api"

type ServerStatePathManager struct{}

func (self *ServerStatePathManager) Path() api.PathSpec {
	return api.NewSafeDatastorePath("config", "server_state").SetType("json")
}
