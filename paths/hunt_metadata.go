package paths

import (
	"context"
	"time"

	"www.velocidex.com/golang/velociraptor/file_store/api"
)

type HuntPathManager struct {
	path    api.PathSpec
	hunt_id string
}

func (self HuntPathManager) Path() api.PathSpec {
	return self.path
}

func (self HuntPathManager) GetPathForWriting() (api.PathSpec, error) {
	return self.path, nil
}

func (self HuntPathManager) GetQueueName() string {
	return self.hunt_id
}

// Get the file store path for placing the download zip for the flow.
func (self HuntPathManager) GetHuntDownloadsFile(only_combined bool,
	base_filename string) api.PathSpec {
	suffix := ""
	if only_combined {
		suffix = "-summary"
	}

	return api.NewUnsafeDatastorePath(
		"downloads", "hunts", self.hunt_id,
		base_filename+self.hunt_id+suffix+".zip")
}

func (self HuntPathManager) GetAvailableFiles(
	ctx context.Context) []*api.ResultSetFileProperties {
	return []*api.ResultSetFileProperties{{
		Path:    self.path,
		EndTime: time.Unix(int64(1)<<62, 0),
	}}
}

func NewHuntPathManager(hunt_id string) *HuntPathManager {
	return &HuntPathManager{
		path:    api.NewUnsafeDatastorePath("hunts", hunt_id),
		hunt_id: hunt_id,
	}
}

func (self HuntPathManager) Stats() api.PathSpec {
	return self.path.AddChild("stats")
}

func (self HuntPathManager) HuntDirectory() api.PathSpec {
	return api.NewSafeDatastorePath("hunts")
}

// Get result set for storing participating clients.
func (self HuntPathManager) Clients() api.PathSpec {
	return api.NewSafeDatastorePath("hunts", self.hunt_id).SetType("json")
}

// Where to store client errors.
func (self HuntPathManager) ClientErrors() api.PathSpec {
	return api.NewSafeDatastorePath(
		"hunts", self.hunt_id+"_errors").SetType("json")
}
