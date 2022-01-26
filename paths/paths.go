// This file defines the schema of where various things go into the
// filestore.

package paths

import (
	"regexp"
	"strings"
	"time"
)

const (
	// The different types of artifacts.
	MODE_INVALID = iota
	MODE_CLIENT
	MODE_CLIENT_EVENT
	MODE_SERVER
	MODE_SERVER_EVENT
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

// When an artifact is compiled into VQL, the final query in a source
// sequence is given a name. The result set will carry this name as
// the rows belonging to the named query. QueryNameToArtifactAndSource
// will split the query name into an artifact and source. Some
// artifacts do not have a named source, in which case the source name
// will be ""
func QueryNameToArtifactAndSource(query_name string) (
	artifact_name, artifact_source string) {
	components := strings.Split(query_name, "/")
	switch len(components) {
	case 2:
		return components[0], components[1]
	default:
		return components[0], ""
	}
}

var day_name_regex = regexp.MustCompile(
	`^\d\d\d\d-\d\d-\d\d`)

func DayNameToTimestamp(name string) int64 {
	matches := day_name_regex.FindAllString(name, -1)
	if len(matches) == 1 {
		time, err := time.Parse("2006-01-02 MST",
			matches[0]+" UTC")
		if err == nil {
			return time.Unix()
		}
	}
	return 0
}

var pathSplit_re = regexp.MustCompile(`[/\\]`)

func GenericPathSplit(path string) []string {
	return pathSplit_re.Split(path, -1)
}
