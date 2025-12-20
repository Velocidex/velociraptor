package paths

import (
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
)

func DSPathSpecFromClientPath(client_path string) api.DSPathSpec {
	components := ExtractClientPathComponents(client_path)
	result := path_specs.NewUnsafeDatastorePath(components...)
	if len(components) > 0 {
		last := len(components) - 1
		name_type, name := api.GetDataStorePathTypeFromExtension(
			components[last])
		components[last] = name
		return result.SetType(name_type)
	}
	return result
}

func FSPathSpecFromClientPath(client_path string) api.FSPathSpec {
	components := ExtractClientPathComponents(client_path)
	return path_specs.FromGenericComponentList(components)
}
