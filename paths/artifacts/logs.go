package artifacts

import (
	"context"
	"fmt"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/paths"
)

// A path manager that specifically writes query log files. Controls
// where the logs will be stored.
type ArtifactLogPathManager struct {
	*ArtifactPathManager
}

func (self *ArtifactLogPathManager) Path() string {
	result, _ := self.GetPathForWriting()
	return result
}

// Returns the root path for all day logs. Walking this path will
// produce all logs for this client and all artifacts.
func (self *ArtifactLogPathManager) GetRootPath() string {
	switch self.mode {
	case paths.MODE_CLIENT:
		return fmt.Sprintf(
			"/clients/%s/collections/%s/logs",
			self.client_id, self.flow_id)

	case paths.MODE_SERVER:
		return fmt.Sprintf(
			"/clients/server/collections/%s/logs", self.flow_id)

	case paths.MODE_SERVER_EVENT:
		return "/server_artifact_logs"

	case paths.MODE_CLIENT_EVENT:
		if self.client_id == "" {
			// Should never normally happen.
			return "/clients/nobody"

		} else {
			return fmt.Sprintf("/clients/%s/monitoring_logs",
				self.client_id)
		}
	default:
		return "invalid"
	}
}

func (self *ArtifactLogPathManager) GetPathForWriting() (string, error) {
	switch self.mode {
	case paths.MODE_CLIENT:
		return fmt.Sprintf(
			"/clients/%s/collections/%s/logs",
			self.client_id, self.flow_id), nil

	case paths.MODE_SERVER:
		return fmt.Sprintf(
			"/clients/server/collections/%s/logs", self.flow_id), nil

	case paths.MODE_SERVER_EVENT:
		if self.source != "" {
			return fmt.Sprintf(
				"/server_artifact_logs/%s/%s/%s.json",
				self.base_artifact_name, self.source,
				self.getDayName()), nil
		} else {
			return fmt.Sprintf(
				"/server_artifact_logs/%s/%s.json",
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
					"/clients/%s/monitoring_logs/%s/%s/%s.json",
					self.client_id,
					self.base_artifact_name, self.source,
					self.getDayName()), nil
			} else {
				return fmt.Sprintf(
					"/clients/%s/monitoring_logs/%s/%s.json",
					self.client_id,
					self.base_artifact_name, self.getDayName()), nil
			}
		}

		// Internal artifacts are not written anywhere but are
		// still replicated.
	case paths.INTERNAL:
		return "", nil
	}

	return "", nil
}

func (self *ArtifactLogPathManager) GeneratePaths(
	ctx context.Context) <-chan *api.ResultSetFileProperties {
	output := make(chan *api.ResultSetFileProperties)

	go func() {
		defer close(output)

		path_for_writing, err := self.GetPathForWriting()
		if err != nil {
			return
		}

		// List all daily files in the required directory.
		children, _ := self.get_event_files(path_for_writing)
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
