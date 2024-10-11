package utils

import (
	"fmt"

	"github.com/Velocidex/yaml/v2"
)

// The yaml library is flakey and can sometimes crash on invalid
// input. This wrapper makes sure we dont lose it if the input is not
// valid.
func YamlUnmarshalStrict(data []byte, target interface{}) (err error) {
	defer func() {
		r := recover()
		if r != nil {
			err = fmt.Errorf("Invalid YAML file.")
		}
	}()

	return yaml.UnmarshalStrict(data, target)
}
