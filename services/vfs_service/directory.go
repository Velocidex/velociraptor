package vfs_service

import (
	"context"
	"errors"
	"io"

	"github.com/Velocidex/ordereddict"
	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/api/tables"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
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
	OSPath    string                       `json:"_OSPath"`
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

	// Empty responses mean the directory is empty - no need to
	// worry about downloads.
	if result.TotalRows == 0 {
		return result, nil
	}

	// The artifact that contains the actual data may vary a bit - let
	// the metadata dictate it.
	artifact_name := result.Artifact
	if artifact_name == "" {
		artifact_name = "System.VFS.ListDirectory"
	}

	// Open the original flow result set
	path_manager := artifacts.NewArtifactPathManagerWithMode(
		config_obj, result.ClientId, result.FlowId,
		artifact_name, paths.MODE_CLIENT)

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
	if errors.Is(err, io.EOF) {
		return nil, nil
	}

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
		if !ok || mode == "" || mode[0] != 'd' {
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
	result.Columns = columns
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

	// Only used for the top level directory
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

	// Now get the download version
	file_store_factory := file_store.GetFileStore(config_obj)
	reader, err := result_sets.NewResultSetReader(file_store_factory,
		path_manager.VFSDownloadInfoResultSet(vfs_components))
	if err == nil {
		defer reader.Close()

		// If the result set does not exist, then total rows will be
		// -1.
		total_rows := reader.TotalRows()
		result.DownloadVersion = 0
		if total_rows > 0 {
			result.DownloadVersion = uint64(total_rows)
		}
	}
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

func (self *VFSService) ListDirectoryFiles(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.GetTableRequest) (*api_proto.GetTableResponse, error) {

	stat, err := self.StatDirectory(config_obj, in.ClientId, in.VfsComponents)
	if err != nil {
		return nil, err
	}

	if stat.FlowId == "" {
		return &api_proto.GetTableResponse{}, nil
	}

	table_request := proto.Clone(in).(*api_proto.GetTableRequest)
	table_request.Artifact = stat.Artifact
	if table_request.Artifact == "" {
		table_request.Artifact = "System.VFS.ListDirectory"
	}

	if table_request.Type == "" {
		table_request.Type = "CLIENT"
	}

	// Transform the table into a subsection of the main table.
	table_request.StartIdx = stat.StartIdx
	table_request.EndIdx = stat.EndIdx
	table_request.FlowId = stat.FlowId

	// Get the table possibly applying any table transformations.
	result, err := tables.GetTable(ctx, config_obj, table_request, "")
	if err != nil {
		return nil, err
	}

	index_of_Name := -1
	for idx, column_name := range result.Columns {
		if column_name == "Name" {
			index_of_Name = idx
			break
		}
	}

	// Should not happen - Name is missing from results.
	if index_of_Name < 0 {
		return result, nil
	}

	// Find all the downloads in this directory, so we can enrich the
	// result with the data.
	lookup := getDirectoryDownloadInfo(
		ctx, config_obj, in.ClientId, in.VfsComponents)
	for _, row := range result.Rows {
		var row_data []interface{}
		err := json.Unmarshal([]byte(row.Json), &row_data)
		if err != nil {
			continue
		}

		if len(row_data) <= index_of_Name {
			continue
		}

		// Find the Name column entry in each cell.
		name, ok := row_data[index_of_Name].(string)
		if !ok || name == "" {
			continue
		}

		// Insert a Download info column in the begining.
		row_data = append([]interface{}{""}, row_data...)

		download_info, pres := lookup[name]
		if pres {
			row_data[0] = download_info
		}

		serialized, err := json.Marshal(row_data)
		if err != nil {
			continue
		}
		row.Json = string(serialized)
	}
	result.Columns = append([]string{"Download"}, result.Columns...)
	return result, nil
}

func getDirectoryDownloadInfo(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string,
	vfs_components []string) map[string]*flows_proto.VFSDownloadInfo {
	result := make(map[string]*flows_proto.VFSDownloadInfo)

	path_manager := paths.NewClientPathManager(client_id)
	file_store_factory := file_store.GetFileStore(config_obj)
	reader, err := result_sets.NewResultSetReader(file_store_factory,
		path_manager.VFSDownloadInfoResultSet(vfs_components))
	if err != nil {
		return result
	}
	defer reader.Close()

	for row := range reader.Rows(ctx) {
		serialized, err := row.MarshalJSON()
		if err != nil {
			continue
		}

		item := &flows_proto.VFSDownloadInfo{}
		err = json.Unmarshal(serialized, item)
		if err != nil {
			continue
		}

		if item.Name == "" {
			continue
		}

		result[item.Name] = item
	}

	return result
}
