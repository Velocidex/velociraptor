package result_sets

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

func (self HuntPathManager) GetArtifact() string {
	return ""
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
		path:    path.Join("/hunts", hunt_id+".json"),
		hunt_id: hunt_id,
	}
}
