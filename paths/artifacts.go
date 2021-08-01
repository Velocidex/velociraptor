package paths

import (
	"strings"

	"www.velocidex.com/golang/velociraptor/file_store/api"
)

// Convert an artifact name to a path component to store its definition.
func GetArtifactDefintionPath(name string) api.UnsafeDatastorePath {
	return ARTIFACT_DEFINITION_PREFIX.AsUnsafe().AddChild(
		strings.Split(name, ".")...).SetFileExtension(".yaml")
}
