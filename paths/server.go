package paths

type ServerStatePathManager struct{}

func (self *ServerStatePathManager) Path() string {
	return "/config/server_state.json"
}
