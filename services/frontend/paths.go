package frontend

type FrontendPathManager struct{}

func (self FrontendPathManager) Path() string {
	return "/frontends"
}

func (self FrontendPathManager) Frontend(name string) string {
	return "/frontends/" + name + ".json"
}
