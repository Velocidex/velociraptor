package artifacts

import (
	"fmt"
	"strings"
	"time"
)

const (
	// The different types of artifacts.
	MODE_CLIENT           = 1
	MODE_SERVER_EVENT     = 2
	MODE_MONITORING_DAILY = 3
	MODE_JOURNAL_DAILY    = 4
)

func ModeNameToMode(name string) int {
	name = strings.ToUpper(name)
	switch name {
	case "CLIENT":
		return MODE_CLIENT
	case "SERVER_EVENT":
		return MODE_SERVER_EVENT
	case "MONITORING_DAILY", "CLIENT_EVENT":
		return MODE_MONITORING_DAILY
	case "JOURNAL_DAILY":
		return MODE_JOURNAL_DAILY
	}
	return 0
}

// Convert an artifact name to a path to store its definition.
func NameToPath(name string) string {
	return "/" + strings.Replace(name, ".", "/", -1) + ".yaml"
}

// Resolve the path relative to the filestore where the CVS files are
// stored. This depends on what kind of log it is (mode), and various
// other details depending on the mode.
//
// This function represents a map between the type of artifact and its
// location on disk. It is used by all code that needs to read or
// write artifact results.
func GetCSVPath(
	client_id, day_name, flow_id, artifact_name, source_name string,
	mode int) string {

	switch mode {
	case MODE_CLIENT:
		if source_name != "" {
			return fmt.Sprintf(
				"/clients/%s/artifacts/%s/%s/%s.csv",
				client_id, artifact_name,
				flow_id, source_name)
		} else {
			return fmt.Sprintf(
				"/clients/%s/artifacts/%s/%s.csv",
				client_id, artifact_name,
				flow_id)
		}

	case MODE_SERVER_EVENT:
		if source_name != "" {
			return fmt.Sprintf(
				"/server_artifacts/%s/%s/%s.csv",
				artifact_name, day_name, source_name)
		} else {
			return fmt.Sprintf(
				"/server_artifacts/%s/%s.csv",
				artifact_name, day_name)
		}

	case MODE_JOURNAL_DAILY:
		if source_name != "" {
			return fmt.Sprintf(
				"/journals/%s/%s/%s.csv",
				artifact_name, day_name, source_name)
		} else {
			return fmt.Sprintf(
				"/journals/%s/%s.csv",
				artifact_name, day_name)
		}

	case MODE_MONITORING_DAILY:
		if client_id == "" {
			return GetCSVPath(
				client_id, day_name,
				flow_id, artifact_name,
				source_name, MODE_JOURNAL_DAILY)

		} else {
			if source_name != "" {
				return fmt.Sprintf(
					"/clients/%s/monitoring/%s/%s/%s.csv",
					client_id, artifact_name,
					day_name, source_name)
			} else {
				return fmt.Sprintf(
					"/clients/%s/monitoring/%s/%s.csv",
					client_id, artifact_name, day_name)
			}
		}
	}

	return ""
}

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

func GetDayName() string {
	now := time.Now()
	return fmt.Sprintf("%d-%02d-%02d", now.Year(),
		now.Month(), now.Day())
}
