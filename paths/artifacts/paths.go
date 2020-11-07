package artifacts

import (
	"context"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
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

	Clock      utils.Clock
	file_store api.FileStore
}

func NewArtifactPathManager(
	config_obj *config_proto.Config,
	client_id, flow_id, full_artifact_name string) *ArtifactPathManager {
	file_store_factory := file_store.GetFileStore(config_obj)
	return &ArtifactPathManager{
		config_obj:         config_obj,
		client_id:          client_id,
		flow_id:            flow_id,
		full_artifact_name: full_artifact_name,
		Clock:              utils.RealClock{},
		file_store:         file_store_factory,
	}
}

func (self *ArtifactPathManager) GetQueueName() string {
	return self.full_artifact_name
}

func (self *ArtifactPathManager) Path() string {
	result, _ := self.GetPathForWriting()
	return result
}

func (self *ArtifactPathManager) GetPathForWriting() (string, error) {
	artifact_name, artifact_source := paths.SplitFullSourceName(self.full_artifact_name)
	mode, err := GetArtifactMode(self.config_obj, artifact_name)
	if err != nil {
		return "", err
	}

	now := self.Clock.Now().UTC()
	day_name := fmt.Sprintf("%d-%02d-%02d", now.Year(),
		now.Month(), now.Day())

	result := get_back_path(self.client_id, day_name, self.flow_id,
		artifact_name, artifact_source, mode)

	return result + ".json", nil
}

// Get the result set files for event artifacts by listing the
// directory that contains all the daily files. NOTE: This is mostly
// used by the DirectoryFileStore to avoid having to scan large event
// files for small time ranges. The SqlFileStore does not need to do
// this because there is a timestamp index on the events themselves.
func (self *ArtifactPathManager) get_event_files() ([]*api.ResultSetFileProperties, error) {
	artifact_name, source_name := paths.SplitFullSourceName(self.full_artifact_name)
	mode, err := GetArtifactMode(self.config_obj, self.full_artifact_name)
	if err != nil {
		return nil, err
	}

	dir_name := ""

	switch mode {
	case paths.MODE_SERVER_EVENT:
		if source_name != "" {
			dir_name = fmt.Sprintf(
				"/server_artifacts/%s/%s/",
				artifact_name, source_name)
		} else {
			dir_name = fmt.Sprintf(
				"/server_artifacts/%s/",
				artifact_name)
		}

	case paths.MODE_CLIENT_EVENT:
		if self.client_id == "" {
			return nil, errors.New("Client event without client id")

		} else {
			if source_name != "" {
				dir_name = fmt.Sprintf(
					"/clients/%s/monitoring/%s/%s",
					self.client_id, artifact_name, source_name)
			} else {
				dir_name = fmt.Sprintf(
					"/clients/%s/monitoring/%s",
					self.client_id, artifact_name)
			}
		}

	default:
		path_for_writing, err := self.GetPathForWriting()
		if err != nil {
			return nil, err
		}
		return []*api.ResultSetFileProperties{
			&api.ResultSetFileProperties{
				Path: path_for_writing,
			}}, nil
	}

	children, err := self.file_store.ListDirectory(dir_name)
	if err != nil {
		return nil, err
	}

	result := make([]*api.ResultSetFileProperties, 0, len(children))
	for _, child := range children {
		full_path := path.Join(dir_name, child.Name())
		timestamp := DayNameToTimestamp(full_path)
		result = append(result, &api.ResultSetFileProperties{
			Path:      full_path,
			StartTime: timestamp,
			EndTime:   timestamp + 60*60*24,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].StartTime < result[j].StartTime
	})

	return result, nil
}

func (self *ArtifactPathManager) GeneratePaths(ctx context.Context) <-chan *api.ResultSetFileProperties {
	output := make(chan *api.ResultSetFileProperties)

	go func() {
		defer close(output)

		// Find the directory over which we need to list.
		children, _ := self.get_event_files()
		for _, child := range children {
			select {
			case <-ctx.Done():
				return

			case output <- child:
			}
		}
	}()

	return output
}

var day_name_regex = regexp.MustCompile(
	`\d\d\d\d-\d\d-\d\d`)

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

// Resolve the path relative to the filestore where the CVS files are
// stored. This depends on what kind of log it is (mode), and various
// other details depending on the mode.
//
// This function represents a map between the type of artifact and its
// location on disk. It is used by all code that needs to read or
// write artifact results.
func get_back_path(client_id, day_name, flow_id, artifact_name, source_name string,
	mode int) string {

	switch mode {
	case paths.MODE_CLIENT:
		if source_name != "" {
			return fmt.Sprintf(
				"/clients/%s/artifacts/%s/%s/%s",
				client_id, artifact_name,
				flow_id, source_name)
		} else {
			return fmt.Sprintf(
				"/clients/%s/artifacts/%s/%s",
				client_id, artifact_name,
				flow_id)
		}

	case paths.MODE_SERVER:
		if source_name != "" {
			return fmt.Sprintf(
				"/clients/server/artifacts/%s/%s/%s",
				artifact_name, flow_id, source_name)
		} else {
			return fmt.Sprintf(
				"/clients/server/artifacts/%s/%s",
				artifact_name, flow_id)
		}

	case paths.MODE_SERVER_EVENT:
		if source_name != "" {
			return fmt.Sprintf(
				"/server_artifacts/%s/%s/%s",
				artifact_name, source_name, day_name)
		} else {
			return fmt.Sprintf(
				"/server_artifacts/%s/%s",
				artifact_name, day_name)
		}

	case paths.MODE_CLIENT_EVENT:
		if client_id == "" {
			// Should never normally happen.
			return fmt.Sprintf(
				"/clients/nobody/monitoring/%s/%s",
				artifact_name, day_name)

		} else {
			if source_name != "" {
				return fmt.Sprintf(
					"/clients/%s/monitoring/%s/%s/%s",
					client_id, artifact_name, source_name,
					day_name)
			} else {
				return fmt.Sprintf(
					"/clients/%s/monitoring/%s/%s",
					client_id, artifact_name, day_name)
			}
		}
	}

	return ""
}

type MonitoringArtifactPathManager struct {
	path string
}

func (self MonitoringArtifactPathManager) Path() string {
	return self.path
}

// Represents the directory where all the available monitoring logs
// are present - i.e. listing this directory reveals all logs
// currently available.
func NewMonitoringArtifactPathManager(client_id string) *MonitoringArtifactPathManager {
	result := &MonitoringArtifactPathManager{}

	if client_id != "" || client_id == "server" {
		result.path = path.Join("/clients", client_id, "monitoring")
	} else {
		result.path = "/server_artifacts"
	}

	return result
}

func GetArtifactMode(config_obj *config_proto.Config, artifact_name string) (int, error) {
	manager, err := services.GetRepositoryManager()
	if err != nil {
		return 0, err
	}

	repository, _ := manager.GetGlobalRepository(config_obj)

	artifact, pres := repository.Get(config_obj, artifact_name)
	if !pres {
		return 0, fmt.Errorf("Artifact %s not known", artifact_name)
	}

	return paths.ModeNameToMode(artifact.Type), nil
}
