package paths

import (
	"context"
	"fmt"
	"path"

	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
)

type ClientPathManager struct {
	path      string
	client_id string
}

func (self ClientPathManager) Path() string {
	return self.path
}

func (self ClientPathManager) GetPathForWriting() (string, error) {
	return self.path, nil
}

func (self ClientPathManager) GetQueueName() string {
	return self.client_id
}

func (self ClientPathManager) GeneratePaths(ctx context.Context) <-chan *api.ResultSetFileProperties {
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

func NewClientPathManager(client_id string) *ClientPathManager {
	return &ClientPathManager{
		path:      path.Join("/clients", client_id),
		client_id: client_id,
	}
}

// Gets the flow's log file.
func (self ClientPathManager) Ping() *ClientPathManager {
	self.path = path.Join(self.path, "ping")
	return &self
}

func (self ClientPathManager) Key() *ClientPathManager {
	self.path = path.Join(self.path, "key")
	return &self
}

func (self ClientPathManager) TasksDirectory() *ClientPathManager {
	self.path = path.Join(self.path, "tasks")
	return &self
}

func (self ClientPathManager) Task(task_id uint64) *ClientPathManager {
	self.path = path.Join(self.path, "tasks", fmt.Sprintf("%d", task_id))
	return &self
}

func (self ClientPathManager) VFSPath(vfs_components []string) string {
	return utils.JoinComponents(append([]string{
		"clients", self.client_id, "vfs"}, vfs_components...), "/")
}

func (self ClientPathManager) VFSDownloadInfoPath(vfs string) string {
	return path.Join("clients", self.client_id, "vfs_files", vfs)
}
