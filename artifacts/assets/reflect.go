package assets

import (
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/utils/yaml"
)

// Loads the api description from the embedded asset
func LoadApiDescription() ([]*api_proto.Completion, error) {
	data, err := ReadFile("/docs/references/vql.yaml")
	if err != nil {
		return nil, err
	}

	result := []*api_proto.Completion{}
	err = yaml.Unmarshal(data, &result)
	return result, err
}
