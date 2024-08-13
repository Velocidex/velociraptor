package paths

import (
	"errors"
	"fmt"

	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/utils"
)

type ClientPathManager struct {
	root      api.DSPathSpec
	client_id string
}

// Where we store client records in datastore.
func (self ClientPathManager) Path() api.DSPathSpec {
	return self.root.SetTag("ClientInfo")
}

func (self ClientPathManager) FlowIndex() api.FSPathSpec {
	return self.root.AddChild("flow_index").AsFilestorePath().
		SetType(api.PATH_TYPE_FILESTORE_JSON).
		SetTag("FlowIndex")
}

func NewClientPathManager(client_id string) *ClientPathManager {
	return &ClientPathManager{
		root:      CLIENTS_ROOT.AddChild(client_id),
		client_id: client_id,
	}
}

// We store the last time we saw the client in this location.
func (self ClientPathManager) Ping() api.DSPathSpec {
	return self.root.AddChild("ping").
		SetType(api.PATH_TYPE_DATASTORE_JSON).
		SetTag("ClientPing")
}

// Keep a record of all the client's labels.
func (self ClientPathManager) Labels() api.DSPathSpec {
	return self.root.AddChild("labels").
		SetType(api.PATH_TYPE_DATASTORE_JSON).
		SetTag("ClientLabels")
}

// Each client can have arbitrary key/value metadata.
func (self ClientPathManager) Metadata() api.DSPathSpec {
	return self.root.AddChild("metadata").
		SetType(api.PATH_TYPE_DATASTORE_JSON)
}

// Store each client's public key so we can communicate with it.
func (self ClientPathManager) Key() api.DSPathSpec {
	return self.root.AddChild("key").
		SetTag("ClientKey")
}

// Queue tasks for the client in a directory within the client's main directory.
func (self ClientPathManager) TasksDirectory() api.DSPathSpec {
	return self.root.AddChild("tasks").
		SetTag("ClientTaskQueue")
}

// Store each task within the tasks directory.
func (self ClientPathManager) Task(task_id uint64) api.DSPathSpec {
	return self.root.AddChild("tasks", fmt.Sprintf("%d", task_id)).
		SetTag("ClientTask")
}

// Where we store client VFS information - depends on client paths.
func (self ClientPathManager) VFSPath(vfs_components []string) api.DSPathSpec {
	return CLIENTS_ROOT.AddUnsafeChild(self.client_id, "vfs").
		AddChild(vfs_components...).
		SetType(api.PATH_TYPE_DATASTORE_JSON).
		SetTag("VFS")
}

// A PathSpec for reading the client's file store
func (self ClientPathManager) FSItem(components []string) api.FSPathSpec {
	return CLIENTS_ROOT.AddUnsafeChild(self.client_id).
		AddChild(components...).AsFilestorePath()
}

// The client info protobuf stores information about each downloaded
// file in the datastore.
func (self ClientPathManager) VFSDownloadInfoPath(
	vfs_components []string) api.DSPathSpec {
	return CLIENTS_ROOT.AddUnsafeChild(self.client_id, "vfs_files").
		AddChild(vfs_components...).
		SetType(api.PATH_TYPE_DATASTORE_JSON).
		SetTag("VFSFile")
}

// We now write all download mutations into the same result set in the
// vfs_files directory. The server will scan all updates in order and
// keep the last one to get the current status of each file within the
// directory.
func (self ClientPathManager) VFSDownloadInfoResultSet(
	directory_vfs_components []string) api.FSPathSpec {
	return CLIENTS_ROOT.AddUnsafeChild(self.client_id, "vfs_files").
		AddChild(directory_vfs_components...).AsFilestorePath().
		SetTag("VFSDownloadInfoResultSet")
}

// The uploads tab contains the full VFS path. This function parses
// that and returns an FSPathSpec to access uploads.
// We check to make sure the VFS path belongs to this client.
func (self ClientPathManager) GetUploadsFileFromVFSPath(vfs_path string) (
	api.FSPathSpec, error) {
	components := utils.SplitComponents(vfs_path)
	if len(components) < 5 {
		return nil, errors.New("Vfs path is too short")
	}

	if components[0] != "clients" ||
		components[1] != self.client_id ||
		components[2] != "collections" {
		return nil, errors.New("Invalid vfs_path")
	}

	return path_specs.NewUnsafeFilestorePath(components...).
		SetType(api.PATH_TYPE_FILESTORE_ANY), nil
}
