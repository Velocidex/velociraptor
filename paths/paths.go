// This file defines the schema of where various things go into the
// filestore.

package paths

import (
	"strings"
)

// Fully qualified source names are obtained by joining the artifact
// name to the source name. This splits them back up.
func SplitFullSourceName(artifact_source string) (artifact string, source string) {
	parts := strings.Split(artifact_source, "/")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}

	return artifact_source, ""
}

func FullArtifactName(artifact_name, source_name string) string {
	raw_artifact_name, original_source_name := SplitFullSourceName(artifact_name)

	if source_name == "" {
		source_name = original_source_name
	}

	if source_name != "" {
		return raw_artifact_name + "/" + source_name
	}

	return raw_artifact_name
}
