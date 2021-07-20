package artifacts

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

// The path manager is responsible for telling the file store where to
// store the rows.
type ArtifactPathManager struct {
	config_obj                             *config_proto.Config
	client_id, flow_id, full_artifact_name string
	base_artifact_name, source             string
	mode                                   int
	Clock                                  utils.Clock
	file_store                             api.FileStore
}

func NewArtifactPathManager(
	config_obj *config_proto.Config,
	client_id, flow_id, full_artifact_name string) (
	*ArtifactPathManager, error) {
	artifact_name, artifact_source := paths.SplitFullSourceName(full_artifact_name)

	mode, err := GetArtifactMode(config_obj, artifact_name)
	if err != nil {
		return nil, err
	}

	file_store_factory := file_store.GetFileStore(config_obj)
	return &ArtifactPathManager{
		config_obj:         config_obj,
		client_id:          client_id,
		flow_id:            flow_id,
		full_artifact_name: full_artifact_name,
		base_artifact_name: artifact_name,
		source:             artifact_source,
		mode:               mode,
		Clock:              utils.RealClock{},
		file_store:         file_store_factory,
	}, nil
}

func (self *ArtifactPathManager) Logs() *ArtifactLogPathManager {
	return &ArtifactLogPathManager{self}
}

func (self *ArtifactPathManager) GetQueueName() string {
	return self.full_artifact_name
}

func (self *ArtifactPathManager) Path() string {
	result, _ := self.GetPathForWriting()
	return result
}

// Returns the root path for all day logs. Walking this path will
// produce all logs for this client and all artifacts.
func (self *ArtifactPathManager) GetRootPath() string {
	switch self.mode {
	case paths.MODE_CLIENT, paths.MODE_SERVER:
		return fmt.Sprintf("/clients")

	case paths.MODE_SERVER_EVENT:
		return "/server_artifacts"

	case paths.MODE_CLIENT_EVENT:
		if self.client_id == "" {
			// Should never normally happen.
			return "/clients/nobody"

		} else {
			return fmt.Sprintf("/clients/%s/monitoring",
				self.client_id)
		}
	default:
		return "invalid"
	}
}

func (self *ArtifactPathManager) getDayName() string {
	now := self.Clock.Now().UTC()
	return fmt.Sprintf("%d-%02d-%02d", now.Year(),
		now.Month(), now.Day())
}

// Resolve the path relative to the filestore where the JSONL files
// are stored. This depends on what kind of log it is (mode), and
// various other details depending on the mode.
//
// This function represents a map between the type of artifact and its
// location on disk. It is used by all code that needs to read or
// write artifact results.
func (self *ArtifactPathManager) GetPathForWriting() (string, error) {
	switch self.mode {
	case paths.MODE_CLIENT:
		if self.source != "" {
			return fmt.Sprintf(
				"/clients/%s/artifacts/%s/%s/%s.json",
				self.client_id, self.base_artifact_name,
				self.flow_id, self.source), nil
		} else {
			return fmt.Sprintf(
				"/clients/%s/artifacts/%s/%s.json",
				self.client_id, self.base_artifact_name,
				self.flow_id), nil
		}

	case paths.MODE_SERVER:
		if self.source != "" {
			return fmt.Sprintf(
				"/clients/server/artifacts/%s/%s/%s.json",
				self.base_artifact_name,
				self.flow_id, self.source), nil
		} else {
			return fmt.Sprintf(
				"/clients/server/artifacts/%s/%s.json",
				self.base_artifact_name, self.flow_id), nil
		}

	case paths.MODE_SERVER_EVENT:
		if self.source != "" {
			return fmt.Sprintf(
				"/server_artifacts/%s/%s/%s.json",
				self.base_artifact_name, self.source,
				self.getDayName()), nil
		} else {
			return fmt.Sprintf(
				"/server_artifacts/%s/%s.json",
				self.base_artifact_name, self.getDayName()), nil
		}

	case paths.MODE_CLIENT_EVENT:
		if self.client_id == "" {
			// Should never normally happen.
			return fmt.Sprintf(
				"/clients/nobody/%s/%s.json",
				self.base_artifact_name, self.getDayName()), nil

		} else {
			if self.source != "" {
				return fmt.Sprintf(
					"/clients/%s/monitoring/%s/%s/%s.json",
					self.client_id,
					self.base_artifact_name, self.source,
					self.getDayName()), nil
			} else {
				return fmt.Sprintf(
					"/clients/%s/monitoring/%s/%s.json",
					self.client_id, self.base_artifact_name,
					self.getDayName()), nil
			}
		}

		// Internal artifacts are not written anywhere but are
		// still replicated.
	case paths.INTERNAL:
		return "", nil
	}

	return "", nil
}

// Get the result set files for event artifacts by listing the
// directory that contains all the daily files.
func (self *ArtifactPathManager) get_event_files(path_for_writing string) (
	[]*api.ResultSetFileProperties, error) {

	switch self.mode {
	case paths.MODE_SERVER_EVENT, paths.MODE_CLIENT_EVENT:
	case paths.MODE_CLIENT, paths.MODE_SERVER:
		return []*api.ResultSetFileProperties{
			&api.ResultSetFileProperties{
				Path: path_for_writing,
			}}, nil

	default:
		return nil, nil
	}

	dir_name := path.Dir(path_for_writing)
	children, err := self.file_store.ListDirectory(dir_name)
	if err != nil {
		return nil, err
	}
	result := make([]*api.ResultSetFileProperties, 0, len(children))
	for _, child := range children {
		full_path := path.Join(dir_name, child.Name())
		if !strings.HasSuffix(full_path, ".json") {
			continue
		}

		timestamp := DayNameToTimestamp(full_path)
		result = append(result, &api.ResultSetFileProperties{
			Path:      full_path,
			StartTime: timestamp,
			EndTime:   timestamp.Add(24 * time.Hour),
			Size:      child.Size(),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].StartTime.Before(result[j].StartTime)
	})
	return result, nil
}

func (self *ArtifactPathManager) GetAvailableFiles(
	ctx context.Context) []*api.ResultSetFileProperties {

	path_for_writing, err := self.GetPathForWriting()
	if err != nil {
		return nil
	}

	// Find the directory over which we need to list.
	children, _ := self.get_event_files(path_for_writing)
	return children
}

var day_name_regex = regexp.MustCompile(`\d\d\d\d-\d\d-\d\d`)

func DayNameToTimestamp(name string) time.Time {
	matches := day_name_regex.FindAllString(name, -1)
	if len(matches) == 1 {
		time, err := time.Parse("2006-01-02 MST",
			matches[0]+" UTC")
		if err == nil {
			return time
		}
	}
	return time.Time{}
}

func GetArtifactMode(config_obj *config_proto.Config, artifact_name string) (int, error) {
	manager, err := services.GetRepositoryManager()
	if err != nil {
		return 0, err
	}

	repository, _ := manager.GetGlobalRepository(config_obj)

	artifact_type, err := repository.GetArtifactType(config_obj, artifact_name)
	if err != nil {
		return 0, err
	}

	return paths.ModeNameToMode(artifact_type), nil
}
