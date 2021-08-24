/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
/*

The Virtual Filesystem is a convenient place to collect a lot of
information about the client.  The implementation of the VFS depends
on what kind of information is stored within it.

We select the correct VFS driver based on the first path component.

The VFS is stored in the data store as an abstraction, linking back to
data that was previously obtained by collecting regular artifacts.

There are two kinds of data information:

1. The VFSListResponse stores information about a single directory. It
   contains a copy of the directory listing, as well as a reference to
   the flow that actually collected the data.

2. The VFSDownloadInfo protobuf stores metadata about a bulk file
   download, including its download time and the vfs path which
   actually contains its data (normally this will be inside the flow
   which uploaded it).


VFSListResponse protobufs are stored in:

<filestore>/clients/<client_id>/vfs/<directory path>.db

Each such protobuf contains the listing of all files inside this
directory.


VFSDownloadInfo protobufs are stored in:

<filestore>/clients/<client_id>/vfs_files/<file path>.db


NOTE: The GUI sees files as they appear to the client
(i.e. client_paths) because the GUI reads the output of the
artifacts. These may not be representable in the file store and so
they may be escaped. Therefore we are careful to convert client paths
to vfs path using GetVFSDirectoryInfoPath() and
GetVFSDownloadInfoPath().

*/
package api

import (
	"fmt"
	"strings"

	context "golang.org/x/net/context"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	datastore "www.velocidex.com/golang/velociraptor/datastore"
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
    {"Mode": "drwxrwxrwx", "Name": "file"},
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

func vfsListDirectory(
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
func vfsStatDirectory(
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

func vfsStatDownload(
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

// Split the vfs path into a client path and an accessor. We only
// support certain well defined prefixes which control the type of
// accessor to use.

// The GUI uses a VFS path but the client does not know about how the
// GUI organizes files. In the GUI, files are organized in a tree,
// where the top level directory is the accessor, the rest of the path
// is passed to the accessor directly.
func GetClientPath(components []string) (client_path string, accessor string) {
	if len(components) == 0 {
		return "", "file"
	}

	switch components[0] {
	case "file", "registry":
		return utils.JoinComponents(components[1:], "/"), components[0]

	case "ntfs":
		// With the ntfs accessor, first component is a device
		// and should not be preceded with /
		return strings.Join(components[1:], "\\"), components[0]

	default:
		// This should not happen - try to get it using file accessor.
		return utils.JoinComponents(components[1:], "/"), components[0]
	}
}

func vfsRefreshDirectory(
	self *ApiServer,
	ctx context.Context,
	client_id string,
	vfs_components []string,
	depth uint64) (*flows_proto.ArtifactCollectorResponse, error) {

	client_path, accessor := GetClientPath(vfs_components)
	request := MakeCollectorRequest(
		client_id, "System.VFS.ListDirectory",
		"Path", client_path,
		"Accessor", accessor,
		"Depth", fmt.Sprintf("%v", depth))

	// VFS navigation is high priority.
	request.Urgent = true

	result, err := self.CollectArtifact(ctx, request)
	return result, err
}
