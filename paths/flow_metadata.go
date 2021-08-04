package paths

import (
	"fmt"

	"www.velocidex.com/golang/velociraptor/file_store/api"
)

// Represents paths for storing an uploaded file in the filestore. The
// path incorporates the filename on the client so it is not safe to
// directly use in the file store.
type UploadFile struct {
	path api.PathSpec
}

// Where the uploaded file is stored in the filestore.
func (self UploadFile) Path() api.PathSpec {
	return self.path
}

// Where to write the index path - if the uploaded file is a sparse
// file, an index file will be written with the ranges.
func (self UploadFile) IndexPath() api.PathSpec {
	return self.path.SetType(api.PATH_TYPE_FILESTORE_SPARSE_IDX)
}

// Manage information about each collection.
type FlowPathManager struct {
	client_id string
	flow_id   string
}

func (self FlowPathManager) Path() api.PathSpec {
	return CLIENTS_ROOT.AddChild(self.client_id,
		"collections", self.flow_id)
}

func (self FlowPathManager) ContainerPath() api.PathSpec {
	return CLIENTS_ROOT.AddChild(self.client_id, "collections")
}

func NewFlowPathManager(client_id, flow_id string) *FlowPathManager {
	return &FlowPathManager{
		client_id: client_id,
		flow_id:   flow_id,
	}
}

// Gets the flow's log file.
func (self FlowPathManager) Log() api.PathSpec {
	return self.Path().AddChild("logs").
		SetType(api.PATH_TYPE_FILESTORE_ANY)
}

func (self FlowPathManager) Task() api.PathSpec {
	return self.Path().AddChild("task").
		SetType(api.PATH_TYPE_DATASTORE_PROTO)
}

func (self FlowPathManager) UploadMetadata() api.PathSpec {
	return self.Path().AddChild("uploads").SetType(
		api.PATH_TYPE_FILESTORE_JSON)
}

func (self FlowPathManager) GetDownloadsFile(hostname string) api.PathSpec {
	// If there is no hostname we drop the leading -
	if hostname != "" {
		hostname += "-"
	}
	return DOWNLOADS_ROOT.AddUnsafeChild(self.client_id, self.flow_id,
		fmt.Sprintf("%v%v-%v", hostname, self.client_id, self.flow_id))
}

func (self FlowPathManager) GetReportsFile(hostname string) api.PathSpec {
	// If there is no hostname we drop the leading -
	if hostname != "" {
		hostname += "-"
	}
	return DOWNLOADS_ROOT.AddUnsafeChild(self.client_id, self.flow_id,
		fmt.Sprintf("Report %v%v-%v", hostname,
			self.client_id, self.flow_id)).
		SetType(api.PATH_TYPE_FILESTORE_DOWNLOAD_REPORT)
}

// Where to store the uploaded file in the filestore.
func (self FlowPathManager) GetUploadsFile(
	accessor, client_path string) *UploadFile {
	// Apply the default accessor if not specified.
	if accessor == "" {
		accessor = "file"
	}

	base_path := CLIENTS_ROOT.AddUnsafeChild(self.client_id, "collections",
		self.flow_id, "uploads").
		SetType(api.PATH_TYPE_FILESTORE_ANY)

	return &UploadFile{
		path: UnsafeDatastorePathFromClientPath(
			base_path, accessor, client_path),
	}
}
