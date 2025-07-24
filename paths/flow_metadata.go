package paths

import (
	"fmt"

	"www.velocidex.com/golang/velociraptor/file_store/api"
)

// Represents paths for storing an uploaded file in the filestore. The
// path incorporates the filename on the client so it is not safe to
// directly use in the file store.
type UploadFile struct {
	path        api.FSPathSpec
	client_path string
}

// Where the uploaded file is stored in the filestore.
func (self UploadFile) Path() api.FSPathSpec {
	return self.path
}

// Where to write the index path - if the uploaded file is a sparse
// file, an index file will be written with the ranges.
func (self UploadFile) IndexPath() api.FSPathSpec {
	return self.path.SetType(api.PATH_TYPE_FILESTORE_SPARSE_IDX)
}

func (self UploadFile) VisibleVFSPath() string {
	return self.client_path
}

// Manage information about each collection.
type FlowPathManager struct {
	client_id string
	flow_id   string
}

func (self FlowPathManager) Path() api.DSPathSpec {
	return CLIENTS_ROOT.AddChild(self.client_id,
		"collections", self.flow_id).
		SetType(api.PATH_TYPE_DATASTORE_JSON).
		SetTag("FlowContext")
}

// Store the last update of the flow both in the flow itself, and an
// external file. The real update is the latest of either.
func (self FlowPathManager) Ping() api.DSPathSpec {
	return CLIENTS_ROOT.AddChild(self.client_id,
		"collections", self.flow_id, "ping").
		SetType(api.PATH_TYPE_DATASTORE_JSON).
		SetTag("FlowPing")
}

func (self FlowPathManager) ContainerPath() api.DSPathSpec {
	return CLIENTS_ROOT.AddChild(self.client_id, "collections")
}

func NewFlowPathManager(client_id, flow_id string) *FlowPathManager {
	return &FlowPathManager{
		client_id: client_id,
		flow_id:   flow_id,
	}
}

// Gets the flow's log file.
func (self FlowPathManager) Log() api.FSPathSpec {
	return self.Path().AddChild("logs").
		AsFilestorePath().
		SetTag("Log").
		SetType(api.PATH_TYPE_FILESTORE_JSON)
}

func (self FlowPathManager) LogLegacy() api.FSPathSpec {
	return self.Path().AddChild("logs").
		AsFilestorePath().
		SetType(api.PATH_TYPE_FILESTORE_ANY)
}

func (self FlowPathManager) Task() api.DSPathSpec {
	return self.Path().AddChild("task").
		SetType(api.PATH_TYPE_DATASTORE_PROTO).
		SetTag("FlowTask")
}

func (self FlowPathManager) Stats() api.DSPathSpec {
	return self.Path().AddChild("stats").
		SetType(api.PATH_TYPE_DATASTORE_JSON).
		SetTag("FlowStats")
}

func (self FlowPathManager) UploadMetadata() api.FSPathSpec {
	return self.Path().AddChild("uploads").AsFilestorePath()
}

func (self FlowPathManager) UploadTransactions() api.FSPathSpec {
	return self.Path().AddChild("upload_transactions").AsFilestorePath()
}

func (self FlowPathManager) UploadContainer() api.FSPathSpec {
	return self.Path().AddUnsafeChild("uploads").
		AsFilestorePath().
		SetType(api.PATH_TYPE_FILESTORE_ANY)
}

func (self FlowPathManager) GetDownloadsDirectory() api.FSPathSpec {
	return DOWNLOADS_ROOT.AddUnsafeChild(self.client_id, self.flow_id)
}

func (self FlowPathManager) GetDownloadsFileRawName(filename string) api.FSPathSpec {
	return DOWNLOADS_ROOT.AddUnsafeChild(self.client_id, self.flow_id, filename)
}

func (self FlowPathManager) GetDownloadsStats(
	hostname string, encrypted bool) api.DSPathSpec {
	return self.GetDownloadsFile(hostname, encrypted).
		AsDatastorePath()
}

func (self FlowPathManager) GetDownloadsFile(
	hostname string, encrypted bool) api.FSPathSpec {
	// If there is no hostname we drop the leading -
	if hostname != "" {
		hostname += "-"
	}
	filename := fmt.Sprintf("%v%v-%v", hostname, self.client_id, self.flow_id)
	if encrypted {
		filename += "_locked"
	}
	return DOWNLOADS_ROOT.AddUnsafeChild(self.client_id, self.flow_id, filename)
}

func (self FlowPathManager) GetReportsFile(hostname string) api.FSPathSpec {
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
	accessor,
	client_path string,
	components []string) *UploadFile {
	// Apply the default accessor if not specified.
	if accessor == "" {
		accessor = "file"
	}

	// In case no components were specified, we split the Path into
	// components. Newer clients should send the components list
	// directly to remove splitting ambiguities.
	if components == nil {
		components = ExtractClientPathComponents(client_path)
	}

	base_path := CLIENTS_ROOT.AddUnsafeChild(self.client_id, "collections",
		self.flow_id, "uploads").AsFilestorePath().
		SetType(api.PATH_TYPE_FILESTORE_ANY)

	return &UploadFile{
		client_path: client_path,
		path: base_path.AddUnsafeChild(accessor).
			AddUnsafeChild(components...),
	}
}

// The manager for the flow's notebook
func (self FlowPathManager) Notebook() *NotebookPathManager {
	notebook_id := fmt.Sprintf("N.%v-%v", self.flow_id, self.client_id)
	return NewNotebookPathManager(notebook_id)
}
