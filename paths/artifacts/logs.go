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

func (self *ArtifactLogPathManager) Path() api.FSPathSpec {
	result, _ := self.GetPathForWriting()
	return result
}

// Returns the root path for all day logs. Walking this path will
// produce all logs for this client and all artifacts.
func (self *ArtifactLogPathManager) GetRootPath() api.FSPathSpec {
	switch self.mode {
	case paths.MODE_CLIENT:
		return paths.CLIENTS_ROOT.AddChild(
			self.ClientId, "collections",
			self.FlowId, "logs").AsFilestorePath().
			SetType(api.PATH_TYPE_FILESTORE_JSON)

	case paths.MODE_SERVER, paths.MODE_NOTEBOOK:
		return paths.CLIENTS_ROOT.AddChild(
			"server", "collections",
			self.FlowId, "logs").AsFilestorePath().
			SetType(api.PATH_TYPE_FILESTORE_JSON)

	case paths.MODE_SERVER_EVENT:
		return paths.SERVER_MONITORING_LOGS_ROOT

	case paths.MODE_CLIENT_EVENT:
		if self.ClientId == "" {
			// Should never normally happen.
			return paths.CLIENTS_ROOT.AddChild("nobody").
				AsFilestorePath()

		} else {
			return paths.CLIENTS_ROOT.AddChild(
				self.ClientId, "monitoring_logs").
				AsFilestorePath()
		}
	default:
		return nil
	}
}

func (self *ArtifactLogPathManager) GetPathForWriting() (api.FSPathSpec, error) {
	switch self.mode {
	case paths.MODE_CLIENT:
		return paths.CLIENTS_ROOT.AddChild(
			self.ClientId, "collections",
			self.FlowId, "logs").AsFilestorePath(), nil

	case paths.MODE_SERVER, paths.MODE_NOTEBOOK:
		return paths.CLIENTS_ROOT.AddChild(
			"server", "collections",
			self.FlowId, "logs").AsFilestorePath(), nil

	case paths.MODE_SERVER_EVENT:
		if self.source != "" {
			return paths.SERVER_MONITORING_LOGS_ROOT.AddChild(
				self.base_artifact_name, self.source,
				self.getDayName()), nil
		} else {
			return paths.SERVER_MONITORING_LOGS_ROOT.AddChild(
				self.base_artifact_name, self.getDayName()), nil
		}

	case paths.MODE_CLIENT_EVENT:
		if self.ClientId == "" {
			// Should never normally happen.
			return paths.CLIENTS_ROOT.AddChild(
				"nobody", self.base_artifact_name,
				self.getDayName()).AsFilestorePath(), nil

		} else {
			if self.source != "" {
				return paths.CLIENTS_ROOT.AddChild(
					self.ClientId, "monitoring_logs",
					self.base_artifact_name, self.source,
					self.getDayName()).AsFilestorePath(), nil
			} else {
				return paths.CLIENTS_ROOT.AddChild(
					self.ClientId, "monitoring_logs",
					self.base_artifact_name,
					self.getDayName()).AsFilestorePath(), nil
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
	children, err := self.get_event_files(path_for_writing)
	if err != nil {
		return nil
	}
	return children
}

func NewArtifactLogPathManager(
	ctx context.Context, config_obj *config_proto.Config,
	client_id, flow_id, full_artifact_name string) (
	*ArtifactLogPathManager, error) {

	path_manager, err := NewArtifactPathManager(ctx, config_obj,
		client_id, flow_id, full_artifact_name)
	if err != nil {
		return nil, err
	}

	return &ArtifactLogPathManager{path_manager}, nil
}

func NewArtifactLogPathManagerWithMode(
	config_obj *config_proto.Config,
	client_id, flow_id, full_artifact_name string,
	mode int) *ArtifactLogPathManager {
	path_manager := NewArtifactPathManagerWithMode(config_obj,
		client_id, flow_id, full_artifact_name, mode)
	return &ArtifactLogPathManager{path_manager}
}
