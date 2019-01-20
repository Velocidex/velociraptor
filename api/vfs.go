/*

The Virtual Filesystem is a convenient place to collect a lot of
information about the client.  The implementation of the VFS depends
on what kind of information is stored within it.

We select the correct VFS driver based on the first path component.

*/
package api

import (
	"encoding/json"
	"os"
	"path"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes"
	context "golang.org/x/net/context"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/constants"
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

type FileInfoRow struct {
	Name      string        `json:"Name"`
	Size      int64         `json:"Size"`
	Timestamp time.Time     `json:"Timestamp"`
	Mode      string        `json:"Mode"`
	Download  *DownloadInfo `json:"Download"`
	Mtime     time.Time     `json:"mtime"`
	Atime     time.Time     `json:"atime"`
	Ctime     time.Time     `json:"ctime"`
}

// Render the root level psuedo directory. This provides anchor points
// for the other drivers in the navigation.
func renderRootVFS(client_id string) *actions_proto.VQLResponse {
	if client_id == "" {
		return &actions_proto.VQLResponse{
			Response: `
   [
    {"Mode": "drwxrwxrwx", "Name": "artifact_definitions"},
    {"Mode": "drwxrwxrwx", "Name": "exported_files"}
   ]`,
		}
	}
	return &actions_proto.VQLResponse{
		Response: `
   [
    {"Mode": "drwxrwxrwx", "Name": "file"},
    {"Mode": "drwxrwxrwx", "Name": "ntfs"},
    {"Mode": "drwxrwxrwx", "Name": "registry"},
    {"Mode": "drwxrwxrwx", "Name": "artifacts"},
    {"Mode": "drwxrwxrwx", "Name": "monitoring"}
   ]`,
	}

}

// Render VFS nodes with VQL collection + uploads.
func renderDBVFS(
	config_obj *api_proto.Config,
	client_id string,
	vfs_path string) (*actions_proto.VQLResponse, error) {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	vfs_urn := urns.BuildURN("clients", client_id, "vfs", vfs_path)
	filestore_urn := path.Join("clients", client_id, "vfs_files", vfs_path)
	downloaded_items, err := file_store.GetFileStore(config_obj).
		ListDirectory(filestore_urn)
	if err != nil {
		downloaded_items = []os.FileInfo{}
	}

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
			normalized_name := strings.TrimSuffix(file.Name(), ".gz")
			lookup[normalized_name] = file
		}

		var rows []*FileInfoRow
		err := json.Unmarshal([]byte(result.Response), &rows)
		if err != nil {
			return nil, err
		}

		// If the row refers to a downloaded file, we mark it
		// with the download details.
		for _, row := range rows {
			file, pres := lookup[row.Name]
			if !pres {
				continue
			}

			row.Download = &DownloadInfo{
				VfsPath: path.Join(vfs_path, row.Name),
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

// Render VFS nodes from the filestore.
func renderFileStore(
	config_obj *api_proto.Config,
	prefix string,
	vfs_path string) (*actions_proto.VQLResponse, error) {
	var rows []*FileInfoRow

	filestore_urn := path.Join(prefix, vfs_path)
	items, err := file_store.GetFileStore(config_obj).
		ListDirectory(filestore_urn)
	if err != nil {
		return &actions_proto.VQLResponse{}, nil
	}

	for _, item := range items {
		row := &FileInfoRow{
			Name:      item.Name(),
			Size:      item.Size(),
			Timestamp: item.ModTime(),
		}

		if item.IsDir() {
			row.Mode = "dr-xr-xr-x"
		} else {
			row.Mode = "-r--r--r--"
			row.Download = &DownloadInfo{
				VfsPath: path.Join(vfs_path, item.Name()),
				Size:    item.Size(),
				Mtime:   item.ModTime().UnixNano() / 1000,
			}
		}

		rows = append(rows, row)
	}
	encoded_rows, err := json.MarshalIndent(rows, "", " ")
	if err != nil {
		return nil, err
	}

	result := &actions_proto.VQLResponse{
		Columns: []string{
			"Download", "Name", "Size", "Mode", "Timestamp",
		},
		Response: string(encoded_rows),
		Types: []*actions_proto.VQLTypeMap{
			&actions_proto.VQLTypeMap{
				Column: "Download",
				Type:   "Download",
			},
		},
	}

	return result, nil
}

// We export some paths from the file_store into the VFS. This
// function maps from the browser's vfs view into the file_store
// prefix. If this function returns ok, then the full filestore path
// can be obtained by joining the prefix with the vfs_path provided.
func getVFSPathPrefix(vfs_path string, client_id string) (prefix string, ok bool) {
	if strings.HasPrefix(vfs_path, "/monitoring") ||
		strings.HasPrefix(vfs_path, "/artifacts") {
		return path.Join("/clients", client_id), true
	}

	if strings.HasPrefix(vfs_path, "/artifact_definitions") {
		return "/", true
	}

	if strings.HasPrefix(vfs_path, "/exported_files") {
		return "/", true
	}

	if client_id != "" && strings.HasPrefix(vfs_path, "/clients/"+client_id) {
		return "/", true
	}

	if strings.HasPrefix(vfs_path, "/hunts") && client_id == "" {
		return "/", true
	}

	return "/", false
}

func vfsListDirectory(
	config_obj *api_proto.Config,
	client_id string,
	vfs_path string) (*actions_proto.VQLResponse, error) {
	vfs_path = path.Join("/", vfs_path)

	if vfs_path == "" || vfs_path == "/" {
		return renderRootVFS(client_id), nil
	}

	if vfs_path == "/artifact_definitions" {
		return &actions_proto.VQLResponse{
			Response: `
   [
    {"Mode": "drwxrwxrwx", "Name": "builtin"},
    {"Mode": "drwxrwxrwx", "Name": "custom"}
   ]`,
		}, nil
	}
	if strings.HasPrefix(vfs_path, constants.BUILTIN_ARTIFACT_DEFINITION) {
		return renderBuiltinArtifacts(config_obj, vfs_path)
	}

	prefix, ok := getVFSPathPrefix(vfs_path, client_id)
	if ok {
		return renderFileStore(config_obj, prefix, vfs_path)
	}

	return renderDBVFS(config_obj, client_id, vfs_path)
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
