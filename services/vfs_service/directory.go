package vfs_service

import (
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/utils"
)

type FileInfoRow struct {
	Name      string                       `json:"Name"`
	Size      int64                        `json:"Size"`
	Timestamp string                       `json:"Timestamp"`
	Mode      string                       `json:"Mode"`
	Download  *flows_proto.VFSDownloadInfo `json:"Download"`
	Mtime     string                       `json:"mtime"`
	Atime     string                       `json:"atime"`
	Ctime     string                       `json:"ctime"`
	FullPath  string                       `json:"_FullPath"`
	Data      interface{}                  `json:"_Data"`
}

// Render the root level pseudo directory. This provides anchor points
// for the other drivers in the navigation.
func renderRootVFS(client_id string) *api_proto.VFSListResponse {
	return &api_proto.VFSListResponse{
		Response: `
   [
    {"Mode": "drwxrwxrwx", "Name": "auto"},
    {"Mode": "drwxrwxrwx", "Name": "ntfs"},
    {"Mode": "drwxrwxrwx", "Name": "registry"}
   ]`,
	}
}

// Render VFS nodes with VQL collection + uploads.
func renderDBVFS(
	config_obj *config_proto.Config,
	client_id string,
	components []string) (*api_proto.VFSListResponse, error) {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	path_manager := paths.NewClientPathManager(client_id)

	// Figure out where the download info files are.
	download_info_path := path_manager.VFSDownloadInfoPath(components)
	downloaded_files, _ := db.ListChildren(config_obj, download_info_path)

	result := &api_proto.VFSListResponse{}

	// Figure out where the directory info is.
	vfs_path := path_manager.VFSPath(components)

	// If file does not exist, we have an empty response
	_ = db.GetSubject(config_obj, vfs_path, result)

	// Empty responses mean the directory is empty - no need to
	// worry about downloads.
	json_response := result.Response
	if json_response == "" {
		return result, nil
	}

	// Merge uploaded file info with the VFSListResponse. Note
	// that if there are no downloaded files, we just pass the
	// VFSListResponse lazily to the caller.
	if len(downloaded_files) > 0 {
		lookup := make(map[string]bool)
		for _, filename := range downloaded_files {
			lookup[filename.Base()] = true
		}

		var rows []map[string]interface{}
		err := json.Unmarshal([]byte(json_response), &rows)
		if err != nil {
			return nil, err
		}

		// If the row refers to a downloaded file, we mark it
		// with the download details.
		for _, row := range rows {
			name, ok := row["Name"].(string)
			if !ok {
				continue
			}

			_, pres := lookup[name]
			if !pres {
				continue
			}

			// Make a copy for each path
			file_components := download_info_path.AddChild(name)
			download_info := &flows_proto.VFSDownloadInfo{}
			err := db.GetSubject(
				config_obj, file_components, download_info)
			if err == nil {
				// Support reading older
				// VFSDownloadInfo protobufs which
				// only contained the vfs_path and not
				// the components.
				if download_info.VfsPath != "" {
					download_info.Components = utils.SplitComponents(download_info.VfsPath)
				}

				row["Download"] = download_info
			}
		}

		encoded_rows, err := json.MarshalIndent(rows)
		if err != nil {
			return nil, err
		}

		result.Response = string(encoded_rows)
	}

	// Add a Download column as the first column.
	result.Columns = append([]string{"Download"}, result.Columns...)
	result.Types = append(result.Types, &actions_proto.VQLTypeMap{
		Column: "Download",
		Type:   "Download",
	})

	return result, nil
}

func (self *VFSService) ListDirectory(
	config_obj *config_proto.Config,
	client_id string,
	components []string) (*api_proto.VFSListResponse, error) {

	if len(components) == 0 {
		return renderRootVFS(client_id), nil
	}

	return renderDBVFS(config_obj, client_id, components)
}

// NOTE: We only support stat of DBFS style entries. This function is
// used to track when a directory changes in response to a refresh
// directory flow.
func (self *VFSService) StatDirectory(
	config_obj *config_proto.Config,
	client_id string,
	vfs_components []string) (*api_proto.VFSListResponse, error) {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	path_manager := paths.NewClientPathManager(client_id)
	result := &api_proto.VFSListResponse{}

	// Regardless of error we return success - if the file does
	// not exist yet then it will have no flow id associated with
	// it. This allows the gui to watch for the VFS directory to
	// appear for the first time.
	_ = db.GetSubject(config_obj,
		path_manager.VFSPath(vfs_components), result)

	// Remove the actual response which might be large.
	result.Response = ""

	return result, nil
}

func (self *VFSService) StatDownload(
	config_obj *config_proto.Config,
	client_id string,
	accessor string,
	path_components []string) (*flows_proto.VFSDownloadInfo, error) {

	path_spec := paths.NewClientPathManager(client_id).
		VFSDownloadInfoPath(path_components)

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	result := &flows_proto.VFSDownloadInfo{}

	// Regardless of error we return success - if the file does
	// not exist yet then it will have no flow id associated with
	// it. This allows the gui to watch for the VFS directory to
	// appear for the first time.
	err = db.GetSubject(config_obj, path_spec, result)
	if err != nil {
		return nil, err
	}

	return result, nil
}
