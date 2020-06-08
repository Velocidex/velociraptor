package paths

import (
	"context"
	"path"

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

func (self HuntPathManager) GeneratePaths(ctx context.Context) <-chan *api.ResultSetFileProperties {
	output := make(chan *api.ResultSetFileProperties)
	go func() {
		defer close(output)

		output <- &api.ResultSetFileProperties{
			Path:    self.path,
			EndTime: int64(1) << 62,
		}
	}()
	return output
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
