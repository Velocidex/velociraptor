package paths

import (
	"fmt"

	"www.velocidex.com/golang/velociraptor/file_store/api"
)

type ClientPathManager struct {
	root      api.PathSpec
	client_id string
}

// Where we store client records in datastore.
func (self ClientPathManager) Path() api.PathSpec {
	return self.root
}

func NewClientPathManager(client_id string) *ClientPathManager {
	return &ClientPathManager{
		root:      CLIENTS_ROOT.AddChild(client_id),
		client_id: client_id,
	}
}

// We store the last time we saw the client in this location.
func (self ClientPathManager) Ping() api.PathSpec {
	return self.root.AddChild("ping")
}

// Keep a record of all the client's labels.
func (self ClientPathManager) Labels() api.PathSpec {
	return self.root.AddChild("labels").
		SetType(api.PATH_TYPE_DATASTORE_JSON)
}

// Each client can have arbitrary key/value metadata.
func (self ClientPathManager) Metadata() api.PathSpec {
	return self.root.AddChild("metadata").
		SetType(api.PATH_TYPE_DATASTORE_JSON)
}

// Store each client's public key so we can communicate with it.
func (self ClientPathManager) Key() api.PathSpec {
	return self.root.AddChild("key")
}

// Queue tasks for the client in a directory within the client's main directory.
func (self ClientPathManager) TasksDirectory() api.PathSpec {
	return self.root.AddChild("tasks")
}

// Store each task within the tasks directory.
func (self ClientPathManager) Task(task_id uint64) api.PathSpec {
	return self.root.AddChild("tasks", fmt.Sprintf("%d", task_id))
}

// Where we store client VFS information - depends on client paths.
func (self ClientPathManager) VFSPath(vfs_components []string) api.PathSpec {
	return CLIENTS_ROOT.AddUnsafeChild(self.client_id, "vfs").
		AddChild(vfs_components...)
}

func (self ClientPathManager) VFSDownloadInfoPath(
	vfs_components []string) api.PathSpec {
	return CLIENTS_ROOT.AddUnsafeChild(self.client_id, "vfs_files").
		AddChild(vfs_components...)
}

func (self ClientPathManager) VFSDownloadInfoFromClientPath(
	accessor, client_path string) api.PathSpec {
	base_path := CLIENTS_ROOT.AddUnsafeChild(self.client_id, "vfs_files")

	return UnsafeDatastorePathFromClientPath(
		base_path, accessor, client_path)
}
