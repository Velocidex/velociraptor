package api

import (
	"encoding/json"
	"os"
	"path"

	"github.com/golang/protobuf/ptypes"
	context "golang.org/x/net/context"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config "www.velocidex.com/golang/velociraptor/config"
	datastore "www.velocidex.com/golang/velociraptor/datastore"
	file_store "www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	urns "www.velocidex.com/golang/velociraptor/urns"
)

type DownloadInfo struct {
	VfsPath string `json:"vfs_path"`
	Size    int64  `json:"size"`
	Mtime   int64  `json:"mtime"`
}

func vfsListDirectory(
	config_obj *config.Config,
	client_id string,
	vfs_path string) (*actions_proto.VQLResponse, error) {
	vfs_path = path.Join("/", vfs_path)

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	// If the vfs_path refers to the root directory return the
	// hard coded virtual root.
	virtual_dir_response, pres := getVirtualDirectory(vfs_path)
	if pres {
		return virtual_dir_response, nil
	}

	vfs_urn := urns.BuildURN("clients", client_id, "vfs", vfs_path)
	filestore_urn := path.Join("clients", client_id, "vfs_files", vfs_path)
	downloaded_items, err := file_store.GetFileStore(config_obj).
		ListDirectory(filestore_urn)

	// We only care about actual files.
	downloaded_files := []os.FileInfo{}
	for _, item := range downloaded_items {
		if !item.IsDir() {
			downloaded_files = append(downloaded_files, item)
		}
	}

	result := &actions_proto.VQLResponse{}
	err = db.GetSubject(config_obj, vfs_urn, result)
	if err != nil {
		return nil, err
	}

	// Merge uploaded file info with the VQLResponse. Note that if
	// there are no downloaded files, we just pass the VQLResponse
	// lazily to the caller.
	if len(downloaded_files) > 0 {
		lookup := make(map[string]os.FileInfo)
		for _, file := range downloaded_files {
			lookup[file.Name()] = file
		}

		var rows []map[string]interface{}
		err := json.Unmarshal([]byte(result.Response), &rows)
		if err != nil {
			return nil, err
		}

		// If the row refers to a downloaded file, we mark it
		// with the download details.
		for _, row := range rows {
			filename, pres := row["Name"]
			if !pres {
				continue
			}

			filename_str, ok := filename.(string)
			if !ok {
				continue
			}

			file, pres := lookup[filename_str]
			if !pres {
				continue
			}

			row["Download"] = &DownloadInfo{
				VfsPath: path.Join(vfs_path, filename_str),
				Size:    file.Size(),
				Mtime:   file.ModTime().UnixNano() / 1000,
			}
		}

		encoded_rows, err := json.MarshalIndent(rows, "", " ")
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

func getVirtualDirectory(vfs_path string) (*actions_proto.VQLResponse, bool) {
	if vfs_path == "" || vfs_path == "/" {
		return &actions_proto.VQLResponse{
			Response: `
   [
    {"Mode": "drwxrwxrwx", "Name": "file"},
    {"Mode": "drwxrwxrwx", "Name": "ntfs"},
    {"Mode": "drwxrwxrwx", "Name": "registry"}
   ]`,
		}, true
	}

	return nil, false
}

func vfsRefreshDirectory(
	self *ApiServer,
	ctx context.Context,
	client_id string,
	vfs_path string,
	depth uint64) (*api_proto.StartFlowResponse, error) {

	vfs_path = path.Join("/", vfs_path)
	args := &flows_proto.FlowRunnerArgs{
		ClientId: client_id,
		FlowName: "VFSListDirectory",
	}

	flow_args := &flows_proto.VFSListRequest{
		VfsPath:        vfs_path,
		RecursionDepth: depth,
	}
	any_msg, err := ptypes.MarshalAny(flow_args)
	if err != nil {
		return nil, err
	}

	args.Args = any_msg

	result, err := self.LaunchFlow(ctx, args)
	return result, err
}
