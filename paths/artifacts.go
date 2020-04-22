package paths

import (
	"path"

	"www.velocidex.com/golang/velociraptor/constants"
)

func GetArtifactDefintionPath(name string) string {
	return path.Join(constants.ARTIFACT_DEFINITION_PREFIX, NameToPath(name))
}
