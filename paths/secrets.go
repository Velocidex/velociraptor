package paths

import "www.velocidex.com/golang/velociraptor/file_store/api"

type SecretsPathManager struct{}

func (self SecretsPathManager) SecretsDefinitionDir(type_name string) api.DSPathSpec {
	return CONFIG_ROOT.AddUnsafeChild("secrets", type_name)
}

func (self SecretsPathManager) Secret(type_name, name string) api.DSPathSpec {
	return self.SecretsDefinitionDir(type_name).AddChild(name)
}
