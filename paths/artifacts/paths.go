package artifacts

import (
	"context"
	"fmt"
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

// Used to determine what kind of result set writer is needed. Event
// artifacts need a timed result set but regular artifacts need a
// simple result set.
func (self *ArtifactPathManager) IsEvent() bool {
	switch self.mode {
	// These are regular artifacts
	case paths.MODE_CLIENT, paths.MODE_SERVER:
		return false

		// These are all event artifacts
	case paths.MODE_SERVER_EVENT, paths.MODE_CLIENT_EVENT,
		paths.INTERNAL:
		return true

	default:
		return true
	}
}

// Where we store collection query logs
func (self *ArtifactPathManager) Logs() *ArtifactLogPathManager {
	return &ArtifactLogPathManager{self}
}

func (self *ArtifactPathManager) GetQueueName() string {
	return self.full_artifact_name
}

func (self *ArtifactPathManager) Path() api.FSPathSpec {
	result, _ := self.GetPathForWriting()
	return result
}

// Returns the root path for all day logs. Walking this path will
// produce all logs for this client and all artifacts.
func (self *ArtifactPathManager) GetRootPath() api.FSPathSpec {
	switch self.mode {
	case paths.MODE_CLIENT, paths.MODE_SERVER:
		return paths.CLIENTS_ROOT.AddChild(
			self.client_id, "collections",
			self.flow_id).AsFilestorePath()

	case paths.MODE_SERVER_EVENT:
		return paths.SERVER_MONITORING_ROOT

	case paths.MODE_CLIENT_EVENT:
		if self.client_id == "" {
			// Should never normally happen.
			return paths.CLIENTS_ROOT.AddChild("nobody").
				AsFilestorePath()
		} else {
			return paths.CLIENTS_ROOT.AddChild(
				self.client_id, "monitoring").
				AsFilestorePath()
		}
	default:
		return nil
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
func (self *ArtifactPathManager) GetPathForWriting() (api.FSPathSpec, error) {
	switch self.mode {
	case paths.MODE_CLIENT:
		if self.source != "" {
			return paths.CLIENTS_ROOT.AsFilestorePath().
				SetType(api.PATH_TYPE_FILESTORE_JSON).
				AddChild(
					self.client_id, "artifacts",
					self.base_artifact_name, self.flow_id,
					self.source), nil
		} else {
			return paths.CLIENTS_ROOT.AsFilestorePath().
				SetType(api.PATH_TYPE_FILESTORE_JSON).
				AddChild(
					self.client_id, "artifacts",
					self.base_artifact_name,
					self.flow_id), nil
		}

	case paths.MODE_SERVER:
		if self.source != "" {
			return paths.CLIENTS_ROOT.AsFilestorePath().
				SetType(api.PATH_TYPE_FILESTORE_JSON).
				AddChild(
					"server", "artifacts", self.base_artifact_name,
					self.flow_id, self.source), nil
		} else {
			return paths.CLIENTS_ROOT.AsFilestorePath().
				SetType(api.PATH_TYPE_FILESTORE_JSON).
				AddChild(
					"server", "artifacts", self.base_artifact_name,
					self.flow_id), nil
		}

	case paths.MODE_SERVER_EVENT:
		if self.source != "" {
			return paths.SERVER_MONITORING_ROOT.
				AddChild(
					self.base_artifact_name, self.source,
					self.getDayName()), nil
		} else {
			return paths.SERVER_MONITORING_ROOT.
				AddChild(
					self.base_artifact_name,
					self.getDayName()), nil
		}

	case paths.MODE_CLIENT_EVENT:
		if self.client_id == "" {
			// Should never normally happen.
			return paths.CLIENTS_ROOT.AsFilestorePath().
				SetType(api.PATH_TYPE_FILESTORE_JSON).
				AddChild(
					"nobody", self.base_artifact_name,
					self.getDayName()), nil

		} else {
			if self.source != "" {
				return paths.CLIENTS_ROOT.AsFilestorePath().
					SetType(api.PATH_TYPE_FILESTORE_JSON).
					AddChild(
						self.client_id, "monitoring",
						self.base_artifact_name, self.source,
						self.getDayName()), nil
			} else {
				return paths.CLIENTS_ROOT.AsFilestorePath().
					SetType(api.PATH_TYPE_FILESTORE_JSON).
					AddChild(
						self.client_id, "monitoring",
						self.base_artifact_name,
						self.getDayName()), nil
			}
		}

		// Internal artifacts are not written anywhere but are
		// still replicated.
	case paths.INTERNAL:
		return nil, nil
	}

	return nil, nil
}

// Get the result set files for event artifacts by listing the
// directory that contains all the daily files.
func (self *ArtifactPathManager) get_event_files(path_for_writing api.FSPathSpec) (
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

	dir_name := path_for_writing.Dir()
	children, err := self.file_store.ListDirectory(dir_name)
	if err != nil {
		return nil, err
	}
	result := make([]*api.ResultSetFileProperties, 0, len(children))
	for _, child := range children {
		// We only want to see the JSON files
		if child.PathSpec().Type() != api.PATH_TYPE_FILESTORE_JSON {
			continue
		}

		timestamp := DayNameToTimestamp(child.Name())
		result = append(result, &api.ResultSetFileProperties{
			Path:      child.PathSpec(),
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

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return 0, err
	}

	artifact_type, err := repository.GetArtifactType(config_obj, artifact_name)
	if err != nil {
		return 0, err
	}

	return paths.ModeNameToMode(artifact_type), nil
}
