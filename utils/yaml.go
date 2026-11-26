package utils

import (
	"fmt"

	"go.yaml.in/yaml/v4"
)

// The yaml library is flakey and can sometimes crash on invalid
// input. This wrapper makes sure we don't lose it if the input is not
// valid.
func YamlUnmarshalStrict(data []byte, target interface{}) (err error) {
	defer func() {
		r := recover()
		if r != nil {
			err = fmt.Errorf("Invalid YAML file.")
		}
	}()

	return yaml.Load(data, target,
		yaml.WithV2Defaults(),
		yaml.WithKnownFields(),
		yaml.WithUniqueKeys(),
		yaml.WithSingleDocument())
}
