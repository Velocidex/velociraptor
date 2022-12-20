package vfs_service

import (
	"context"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
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
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string,
	components []string) (*api_proto.VFSListResponse, error) {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	client_path_manager := paths.NewClientPathManager(client_id)

	result := &api_proto.VFSListResponse{}

	// Figure out where the directory info is.
	vfs_path := client_path_manager.VFSPath(components)

	// If file does not exist, we have an empty response otherwise
	// read the response from the db.
	_ = db.GetSubject(config_obj, vfs_path, result)

	// Support deprecated VFS listings protobufs
	if result.Response != "" {
		return renderLegacyDBVFS(config_obj, result, client_id, components)
	}

	// Empty responses mean the directory is empty - no need to
	// worry about downloads.
	if result.TotalRows == 0 {
		return result, nil
	}

	// Open the original flow result set
	path_manager := artifacts.NewArtifactPathManagerWithMode(
		config_obj, result.ClientId, result.FlowId,
		"System.VFS.ListDirectory", paths.MODE_CLIENT)

	file_store_factory := file_store.GetFileStore(config_obj)
	reader, err := result_sets.NewResultSetReader(
		file_store_factory, path_manager.Path())
	if err != nil {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Error("Unable to read artifact: %v", err)
		return result, nil
	}
	defer reader.Close()

	err = reader.SeekToRow(int64(result.StartIdx))
	if err != nil {
		return nil, err
	}

	// If the row refers to a downloaded file, we mark it
	// with the download details.
	count := result.StartIdx
	rows := []*ordereddict.Dict{}
	columns := []string{}

	// Filter the files to produce only the directories. This should
	// be a lot less than total files and so should not take too much
	// memory.
	for row := range reader.Rows(ctx) {
		count++
		if count > result.EndIdx {
			break
		}

		// Only return directories here for the tree widget.
		mode, ok := row.GetString("Mode")
		if !ok || mode[0] != 'd' {
			continue
		}

		rows = append(rows, row)

		if len(columns) == 0 {
			columns = row.Keys()
		}

		// Protect the tree widget from being too large.
		if len(rows) > 2000 {
			break
		}
	}

	encoded_rows, err := json.MarshalIndent(rows)
	if err != nil {
		return nil, err
	}

	result.Response = string(encoded_rows)

	// Add a Download column as the first column.
	result.Columns = columns
	return result, nil
}

// Older versions stored the entire directory listing in the same
// datastore protobuf which makes it very inefficient for large
// directories.
func renderLegacyDBVFS(config_obj *config_proto.Config,
	result *api_proto.VFSListResponse,
	client_id string, components []string) (*api_proto.VFSListResponse, error) {

	client_path_manager := paths.NewClientPathManager(client_id)

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	// Figure out where the download info files are.
	download_info_path := client_path_manager.VFSDownloadInfoPath(components)
	downloaded_files, _ := db.ListChildren(config_obj, download_info_path)

	if len(downloaded_files) == 0 {
		return result, nil
	}

	// Merge uploaded file info with the VFSListResponse.
	lookup := make(map[string]bool)
	for _, filename := range downloaded_files {
		lookup[filename.Base()] = true
	}

	var rows []map[string]interface{}
	err = json.Unmarshal([]byte(result.Response), &rows)
	if err != nil {
		return nil, err
	}

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
	result.Columns = append([]string{"Download"}, result.Columns...)
	return result, nil
}

func (self *VFSService) ListDirectories(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string,
	components []string) (*api_proto.VFSListResponse, error) {

	if len(components) == 0 {
		return renderRootVFS(client_id), nil
	}

	return renderDBVFS(ctx, config_obj, client_id, components)
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
