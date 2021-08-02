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
	return self.path.SetType("idx")
}

// Manage information about each collection.
type FlowPathManager struct {
	client_id string
	flow_id   string
}

func (self FlowPathManager) Path() api.PathSpec {
	return api.NewSafeDatastorePath("clients", self.client_id,
		"collections", self.flow_id).SetType("")
}

func (self FlowPathManager) ContainerPath() api.PathSpec {
	return api.NewSafeDatastorePath("clients", self.client_id, "collections")
}

func NewFlowPathManager(client_id, flow_id string) *FlowPathManager {
	return &FlowPathManager{
		client_id: client_id,
		flow_id:   flow_id,
	}
}

// Gets the flow's log file.
func (self FlowPathManager) Log() api.PathSpec {
	return self.Path().AddChild("logs")
}

func (self FlowPathManager) Task() api.PathSpec {
	return self.Path().AddChild("task")
}

func (self FlowPathManager) UploadMetadata() api.PathSpec {
	return self.Path().AddChild("uploads").SetType("json")
}

func (self FlowPathManager) GetDownloadsFile(hostname string) api.PathSpec {
	// If there is no hostname we drop the leading -
	if hostname != "" {
		hostname += "-"
	}
	return api.NewSafeDatastorePath("downloads", self.client_id,
		self.flow_id).AsUnsafe().AddChild(
		fmt.Sprintf("%v%v-%v.zip", hostname, self.client_id, self.flow_id))
}

func (self FlowPathManager) GetReportsFile(hostname string) api.PathSpec {
	// If there is no hostname we drop the leading -
	if hostname != "" {
		hostname += "-"
	}
	return api.NewSafeDatastorePath("downloads", self.client_id,
		self.flow_id).AsUnsafe().AddChild(
		fmt.Sprintf("Report %v%v-%v.html", hostname, self.client_id, self.flow_id))
}

// Figure out where to store the VFSDownloadInfo file. We maintain a
// metadata file in the client's VFS area linking back to the
// collection which most recently uploaded this file.
func (self FlowPathManager) GetVFSDownloadInfoPath(
	accessor string, client_path string) api.PathSpec {
	base_path := api.NewSafeDatastorePath(
		"clients", self.client_id, "vfs_files", accessor)

	return UnsafeDatastorePathFromClientPath(base_path, accessor, client_path)
}

// GetVFSDownloadInfoPath returns the vfs path to the directory info
// file.
func (self FlowPathManager) GetVFSDirectoryInfoPath(
	accessor, client_path string) *UploadFile {
	base_path := api.NewSafeDatastorePath(
		"clients", self.client_id, "vfs", accessor)

	return &UploadFile{
		path: UnsafeDatastorePathFromClientPath(
			base_path, accessor, client_path),
	}
}

// Where to store the uploaded file in the filestore.
func (self FlowPathManager) GetUploadsFile(
	accessor, client_path string) *UploadFile {
	// Apply the default accessor if not specified.
	if accessor == "" {
		accessor = "file"
	}

	base_path := api.NewSafeDatastorePath(
		"clients", self.client_id, "collections",
		self.flow_id, "uploads", accessor)

	return &UploadFile{
		path: UnsafeDatastorePathFromClientPath(
			base_path, accessor, client_path),
	}
}
