package paths

import (
	"fmt"

	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Represents paths for storing an uploaded file in the filestore. The
// path incorporates the filename on the client so it is not safe to
// directly use in the file store.
type UploadFile struct {
	path api.UnsafeDatastorePath
}

// Where the uploaded file is stored in the filestore.
func (self UploadFile) Path() api.UnsafeDatastorePath {
	return self.path
}

// Where to write the index path - if the uploaded file is a sparse
// file, an index file will be written with the ranges.
func (self UploadFile) IndexPath() api.UnsafeDatastorePath {
	return self.path.SetFileExtension(".idx")
}

// Manage information about each collection.
type FlowPathManager struct {
	client_id string
	flow_id   string
}

func (self FlowPathManager) Path() api.SafeDatastorePath {
	return api.NewSafeDatastorePath("clients", self.client_id,
		"collections", self.flow_id)
}

func (self FlowPathManager) ContainerPath() api.SafeDatastorePath {
	return api.NewSafeDatastorePath("clients", self.client_id, "collections")
}

func NewFlowPathManager(client_id, flow_id string) *FlowPathManager {
	return &FlowPathManager{
		client_id: client_id,
		flow_id:   flow_id,
	}
}

// Gets the flow's log file.
func (self FlowPathManager) Log() api.SafeDatastorePath {
	return self.Path().AddChild("logs")
}

func (self FlowPathManager) Task() api.SafeDatastorePath {
	return self.Path().AddChild("task")
}

func (self FlowPathManager) UploadMetadata() api.SafeDatastorePath {
	return self.Path().AddChild("uploads").SetFileExtension(".json")
}

func (self FlowPathManager) GetDownloadsFile(hostname string) api.UnsafeDatastorePath {
	// If there is no hostname we drop the leading -
	if hostname != "" {
		hostname += "-"
	}
	return api.NewSafeDatastorePath("downloads", self.client_id,
		self.flow_id).AsUnsafe().AddChild(
		fmt.Sprintf("%v%v-%v.zip", hostname, self.client_id, self.flow_id))
}

func (self FlowPathManager) GetReportsFile(hostname string) api.UnsafeDatastorePath {
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
	accessor string, path_components []string) *UploadFile {
	base_path := api.NewSafeDatastorePath(
		"clients", self.client_id, "vfs_files", accessor)

	if accessor == "ntfs" {
		device, subpath_components, err := GetDeviceAndSubpathComponents(
			path_components)
		if err == nil {
			return &UploadFile{
				base_path.AsUnsafe().
					AddChild(device).
					AddChild(subpath_components...),
			}
		}
	}

	return &UploadFile{
		base_path.AsUnsafe().AddChild(path_components...),
	}
}

// GetVFSDownloadInfoPath returns the vfs path to the directory info
// file.
func (self FlowPathManager) GetVFSDirectoryInfoPath(
	accessor, client_path string) *UploadFile {
	base_path := api.NewSafeDatastorePath(
		"clients", self.client_id, "vfs", accessor)

	if accessor == "ntfs" {
		device, subpath, err := GetDeviceAndSubpath(client_path)
		if err == nil {
			return &UploadFile{
				base_path.AsUnsafe().
					AddChild(device).
					AddChild(subpath...),
			}
		}
	}

	return &UploadFile{
		base_path.AsUnsafe().AddChild(
			utils.SplitComponents(client_path)...),
	}
}

// Where to store the uploaded file in the filestore.
func (self FlowPathManager) GetUploadsFile(accessor, client_path string) *UploadFile {
	// Apply the default accessor if not specified.
	if accessor == "" {
		accessor = "file"
	}

	base_path := api.NewSafeDatastorePath(
		"clients", self.client_id, "collections",
		self.flow_id, "uploads", accessor)

	if accessor == "ntfs" {
		device, subpath, err := GetDeviceAndSubpath(client_path)
		if err == nil {
			return &UploadFile{
				base_path.AsUnsafe().
					AddChild(device).
					AddChild(subpath...),
			}
		}
	}

	return &UploadFile{
		base_path.AsUnsafe().AddChild(
			utils.SplitComponents(client_path)...),
	}
}
