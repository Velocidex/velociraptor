package paths

import (
	"context"
	"path"
	"time"

	"www.velocidex.com/golang/velociraptor/file_store/api"
)

type HuntPathManager struct {
	path    string
	hunt_id string
}

func (self HuntPathManager) Path() string {
	return self.path
}

func (self HuntPathManager) GetPathForWriting() (string, error) {
	return self.path, nil
}

func (self HuntPathManager) GetQueueName() string {
	return self.hunt_id
}

// Get the file store path for placing the download zip for the flow.
func (self HuntPathManager) GetHuntDownloadsFile(only_combined bool,
	base_filename string) string {
	suffix := ""
	if only_combined {
		suffix = "-summary"
	}

	return path.Join(
		"/downloads/hunts", self.hunt_id,
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
		path:    path.Join("/hunts", hunt_id),
		hunt_id: hunt_id,
	}
}

func (self HuntPathManager) Stats() *HuntPathManager {
	self.path = path.Join(self.path, "stats")
	return &self
}

func (self HuntPathManager) HuntDirectory() *HuntPathManager {
	self.path = "/hunts"
	return &self
}

// Get result set for storing participating clients.
func (self HuntPathManager) Clients() *HuntPathManager {
	self.path = path.Join("/hunts", self.hunt_id+".json")
	return &self
}

// Where to store client errors.
func (self HuntPathManager) ClientErrors() *HuntPathManager {
	self.path = path.Join("/hunts", self.hunt_id+"_errors.json")
	return &self
}
