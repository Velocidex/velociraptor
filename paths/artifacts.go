package paths

import (
	"path"
	"strings"

	"www.velocidex.com/golang/velociraptor/constants"
)

// Convert an artifact name to a path to store its definition.
func GetArtifactDefintionPath(name string) string {
	return path.Join(constants.ARTIFACT_DEFINITION_PREFIX,
		strings.Replace(name, ".", "/", -1)+".yaml")
}
