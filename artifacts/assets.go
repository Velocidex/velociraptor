// +build release

package artifacts

import (
	"strings"
	"www.velocidex.com/golang/velociraptor/gui/assets"
)

// Load basic artifacts from our assets.
func init() {
	files, err := assets.WalkDirs("", false)
	if err != nil {
		panic(err)
	}

	for _, file := range files {
		if strings.HasPrefix(file, "artifacts/definitions") &&
			strings.HasSuffix(file, "yaml") {
			data, err := assets.ReadFile(file)
			if err != nil {
				continue
			}
			global_repository.LoadYaml(string(data))
		}
	}
}
