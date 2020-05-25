package paths

import (
	"context"
	"path"

	"www.velocidex.com/golang/velociraptor/file_store/api"
)

type InventoryPathManager struct {
	path string
}

func (self InventoryPathManager) Path() string {
	return self.path
}

func (self InventoryPathManager) GetPathForWriting() (string, error) {
	return self.path, nil
}

func (self InventoryPathManager) GetQueueName() string {
	return self.path
}

func (self InventoryPathManager) GeneratePaths(ctx context.Context) <-chan *api.ResultSetFileProperties {
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

func NewInventoryPathManager() *ClientPathManager {
	return &ClientPathManager{
		path: path.Join("/public/inventory.csv"),
	}
}
