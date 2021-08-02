package artifacts

import (
	"context"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/paths"
)

// A path manager that specifically writes query log files. Controls
// where the logs will be stored.
type ArtifactLogPathManager struct {
	*ArtifactPathManager
}

func (self *ArtifactLogPathManager) Path() api.PathSpec {
	result, _ := self.GetPathForWriting()
	return result
}

// Returns the root path for all day logs. Walking this path will
// produce all logs for this client and all artifacts.
func (self *ArtifactLogPathManager) GetRootPath() api.PathSpec {
	switch self.mode {
	case paths.MODE_CLIENT:
		return api.NewUnsafeDatastorePath(
			"clients", self.client_id,
			"collections", self.flow_id, "logs")

	case paths.MODE_SERVER:
		return api.NewUnsafeDatastorePath(
			"clients", "server",
			"collections", self.flow_id, "logs")

	case paths.MODE_SERVER_EVENT:
		return api.NewUnsafeDatastorePath("server_artifact_logs")

	case paths.MODE_CLIENT_EVENT:
		if self.client_id == "" {
			// Should never normally happen.
			return api.NewUnsafeDatastorePath("clients", "nobody")

		} else {
			return api.NewUnsafeDatastorePath(
				"clients", self.client_id, "monitoring_logs")
		}
	default:
		return nil
	}
}

func (self *ArtifactLogPathManager) GetPathForWriting() (api.PathSpec, error) {
	switch self.mode {
	case paths.MODE_CLIENT:
		return api.NewUnsafeDatastorePath(
			"clients", self.client_id,
			"collections", self.flow_id, "logs"), nil

	case paths.MODE_SERVER:
		return api.NewUnsafeDatastorePath(
			"clients", "server",
			"collections", self.flow_id, "logs"), nil

	case paths.MODE_SERVER_EVENT:
		if self.source != "" {
			return api.NewUnsafeDatastorePath(
				"server_artifact_logs",
				self.base_artifact_name, self.source,
				self.getDayName()), nil
		} else {
			return api.NewUnsafeDatastorePath(
				"server_artifact_logs",
				self.base_artifact_name, self.getDayName()), nil
		}

	case paths.MODE_CLIENT_EVENT:
		if self.client_id == "" {
			// Should never normally happen.
			return api.NewUnsafeDatastorePath(
				"clients", "nobody",
				self.base_artifact_name, self.getDayName()), nil

		} else {
			if self.source != "" {
				return api.NewUnsafeDatastorePath(
					"clients", self.client_id,
					"monitoring_logs",
					self.base_artifact_name, self.source,
					self.getDayName()), nil
			} else {
				return api.NewUnsafeDatastorePath(
					"clients", self.client_id,
					"monitoring_logs",
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

func (self *ArtifactLogPathManager) GetAvailableFiles(
	ctx context.Context) []*api.ResultSetFileProperties {
	path_for_writing, err := self.GetPathForWriting()
	if err != nil {
		return nil
	}

	// List all daily files in the required directory.
	children, _ := self.get_event_files(path_for_writing)
	return children
}

func NewArtifactLogPathManager(
	config_obj *config_proto.Config,
	client_id, flow_id, full_artifact_name string) (
	*ArtifactLogPathManager, error) {

	path_manager, err := NewArtifactPathManager(config_obj,
		client_id, flow_id, full_artifact_name)
	if err != nil {
		return nil, err
	}

	return &ArtifactLogPathManager{path_manager}, nil
}
