package paths

import (
	"context"
	"fmt"
	"path"

	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Represents paths for storing an uploaded file in the filestore.
type UploadFile struct {
	path string
}

func (self UploadFile) Path() string {
	return self.path
}

// Where to write the index path
func (self UploadFile) IndexPath() string {
	return self.path + ".idx"
}

type FlowPathManager struct {
	path      string
	client_id string
	flow_id   string
}

func (self FlowPathManager) Path() string {
	return self.path
}

func (self FlowPathManager) ContainerPath() string {
	return path.Join("/clients", self.client_id, "collections")
}

func (self FlowPathManager) GetPathForWriting() (string, error) {
	return self.path, nil
}

func (self FlowPathManager) GetQueueName() string {
	return self.client_id + self.flow_id
}

func (self FlowPathManager) GeneratePaths(ctx context.Context) <-chan *api.ResultSetFileProperties {
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

func NewFlowPathManager(client_id, flow_id string) *FlowPathManager {
	return &FlowPathManager{
		path:      path.Join("/clients", client_id, "collections", flow_id),
		client_id: client_id,
		flow_id:   flow_id,
	}
}

// Gets the flow's log file.
func (self FlowPathManager) Log() *FlowPathManager {
	self.path = path.Join(self.path, "logs")
	return &self
}

func (self FlowPathManager) Task() *FlowPathManager {
	self.path = path.Join(self.path, "task")
	return &self
}

func (self FlowPathManager) UploadMetadata() *FlowPathManager {
	self.path = path.Join(self.path, "uploads.json")
	return &self
}

func (self FlowPathManager) GetDownloadsFile(hostname string) *FlowPathManager {
	// If there is no hostname we drop the leading -
	if hostname != "" {
		hostname += "-"
	}
	self.path = path.Join("/downloads", self.client_id, self.flow_id,
		fmt.Sprintf("%v%v-%v.zip", hostname, self.client_id, self.flow_id))
	return &self
}

func (self FlowPathManager) GetReportsFile(hostname string) *FlowPathManager {
	// If there is no hostname we drop the leading -
	if hostname != "" {
		hostname += "-"
	}
	self.path = path.Join("/downloads", self.client_id, self.flow_id,
		fmt.Sprintf("Report %v%v-%v.html", hostname, self.client_id, self.flow_id))
	return &self
}

// Figure out where to store the VFSDownloadInfo file. We maintain a
// metadata file in the client's VFS area linking back to the
// collection which most recently uploaded this file.
func (self FlowPathManager) GetVFSDownloadInfoPath(
	accessor, client_path string) *FlowPathManager {
	components := []string{"clients", self.client_id, "vfs_files", accessor}

	if accessor == "ntfs" {
		device, subpath, err := GetDeviceAndSubpath(client_path)
		if err == nil {
			components = append(components, device)
			components = append(components, utils.SplitComponents(subpath)...)
			self.path = utils.JoinComponents(components, "/")
			return &self
		}
	}

	components = append(components, utils.SplitComponents(client_path)...)
	self.path = utils.JoinComponents(components, "/")
	return &self
}

// GetVFSDownloadInfoPath returns the vfs path to the directory info
// file.
func (self FlowPathManager) GetVFSDirectoryInfoPath(accessor, client_path string) *FlowPathManager {
	components := []string{"clients", self.client_id, "vfs", accessor}

	if accessor == "ntfs" {
		device, subpath, err := GetDeviceAndSubpath(client_path)
		if err == nil {
			components = append(components, device)
			components = append(components, utils.SplitComponents(subpath)...)
			self.path = utils.JoinComponents(components, "/")
			return &self
		}
	}

	components = append(components, utils.SplitComponents(client_path)...)
	self.path = utils.JoinComponents(components, "/")
	return &self
}

// Currently only CLIENT artifacts upload files. We store the uploaded
// file inside the collection that uploaded it.
func (self FlowPathManager) GetUploadsFile(accessor, client_path string) *UploadFile {
	// Apply the default accessor if not specified.
	if accessor == "" {
		accessor = "file"
	}

	components := []string{
		"clients", self.client_id, "collections",
		self.flow_id, "uploads", accessor}

	if accessor == "ntfs" {
		device, subpath, err := GetDeviceAndSubpath(client_path)
		if err == nil {
			components = append(components, device)
			components = append(components, utils.SplitComponents(subpath)...)
			return &UploadFile{utils.JoinComponents(components, "/")}
		}
	}

	components = append(components, utils.SplitComponents(client_path)...)
	return &UploadFile{utils.JoinComponents(components, "/")}
}
