// This file defines the schema of where various things go into the
// filestore.

package artifacts

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	// The different types of artifacts.
	MODE_CLIENT           = 1
	MODE_SERVER           = 2
	MODE_SERVER_EVENT     = 3
	MODE_MONITORING_DAILY = 4
	MODE_JOURNAL_DAILY    = 5
)

func ModeNameToMode(name string) int {
	name = strings.ToUpper(name)
	switch name {
	case "CLIENT":
		return MODE_CLIENT
	case "SERVER":
		return MODE_SERVER
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

	case MODE_SERVER:
		if source_name != "" {
			return fmt.Sprintf(
				"/clients/server/artifacts/%s/%s/%s.csv",
				artifact_name, flow_id, source_name)
		} else {
			return fmt.Sprintf(
				"/clients/server/artifacts/%s/%s.csv",
				artifact_name, flow_id)
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

// Currently only CLIENT artifacts upload files.
func GetUploadsFile(client_id, flow_id, accessor, client_path string) string {
	return path.Join(
		"/clients", client_id, "collections",
		flow_id, "uploads", accessor,
		utils.Normalize_windows_path(client_path))
}

// GetUploadsMetadata returns the path to the metadata file that contains all the uploads.
func GetUploadsMetadata(client_id, flow_id string) string {
	return path.Join(
		"/clients", client_id, "collections",
		flow_id, "uploads.csv")
}

// Get the file store path for placing the download zip for the flow.
func GetDownloadsFile(client_id, flow_id string) string {
	return path.Join(
		"/downloads", client_id, flow_id,
		flow_id+".zip")
}

func GetArtifactSources(
	config_obj *config_proto.Config,
	artifact string) []string {
	result := []string{}
	repository, err := GetGlobalRepository(config_obj)
	if err == nil {
		artifact_obj, pres := repository.Get(artifact)
		if pres {
			for _, source := range artifact_obj.Sources {
				result = append(result, source.Name)
			}
		}
	}
	return result
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

func GetDayName() string {
	now := time.Now()
	return fmt.Sprintf("%d-%02d-%02d", now.Year(),
		now.Month(), now.Day())
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
