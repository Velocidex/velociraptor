// This file defines the schema of where various things go into the
// filestore.

package paths

import (
	"strings"
)

const (
	// The different types of artifacts.
	MODE_INVALID = iota
	MODE_CLIENT
	MODE_CLIENT_EVENT
	MODE_SERVER
	MODE_SERVER_EVENT
	MODE_NOTEBOOK
	INTERNAL
)

func ModeNameToMode(name string) int {
	name = strings.ToUpper(name)
	switch name {
	case "CLIENT":
		return MODE_CLIENT
	case "CLIENT_EVENT":
		return MODE_CLIENT_EVENT
	case "SERVER":
		return MODE_SERVER
	case "SERVER_EVENT":
		return MODE_SERVER_EVENT
	case "NOTEBOOK":
		return MODE_NOTEBOOK
	case "INTERNAL":
		return INTERNAL
	}
	return MODE_INVALID
}

// Fully qualified source names are obtained by joining the artifact
// name to the source name. This splits them back up.
func SplitFullSourceName(artifact_source string) (artifact string, source string) {
	parts := strings.Split(artifact_source, "/")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}

	return artifact_source, ""
}
