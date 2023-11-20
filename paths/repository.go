package paths

import "www.velocidex.com/golang/velociraptor/file_store/api"

type RepositoryPathManager struct{}

func (self RepositoryPathManager) Metadata() api.DSPathSpec {
	return CONFIG_ROOT.AddChild("repository_metadata")
}
