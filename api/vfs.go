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
	"encoding/json"
	"fmt"
	"strings"

	context "golang.org/x/net/context"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	datastore "www.velocidex.com/golang/velociraptor/datastore"
	file_store "www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
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

// Render the root level psuedo directory. This provides anchor points
// for the other drivers in the navigation.
func renderRootVFS(client_id string) *flows_proto.VFSListResponse {
	return &flows_proto.VFSListResponse{
		Response: `
   [
    {"Mode": "drwxrwxrwx", "Name": "file"},
    {"Mode": "drwxrwxrwx", "Name": "ntfs"},
    {"Mode": "drwxrwxrwx", "Name": "registry"},
    {"Mode": "drwxrwxrwx", "Name": "artifacts"}
   ]`,
	}
}

// Render VFS nodes with VQL collection + uploads.
func renderDBVFS(
	config_obj *config_proto.Config,
	client_id string,
	components []string) (*flows_proto.VFSListResponse, error) {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	// Figure out where the download info files are.
	download_info_path := append([]string{
		"clients", client_id, "vfs_files"}, components...)

	downloaded_files, err := db.ListChildren(
		config_obj, utils.JoinComponents(download_info_path, "/"),
		0, 1000)
	if err != nil {
		return nil, err
	}
	result := &flows_proto.VFSListResponse{}

	// Figure out where the directory info is.
	vfs_path := append([]string{
		"clients", client_id, "vfs"}, components...)
	err = db.GetSubject(
		config_obj, utils.JoinComponents(vfs_path, "/"), result)
	if err != nil {
		return nil, err
	}

	// Empty responses mean the directory is empty - no need to
	// worry about downloads.
	if result.Response == "" {
		return result, nil
	}

	// Merge uploaded file info with the VFSListResponse. Note
	// that if there are no downloaded files, we just pass the
	// VFSListResponse lazily to the caller.
	if len(downloaded_files) > 0 {
		lookup := make(map[string]string)
		for _, filename := range downloaded_files {
			normalized_name := utils.Base(filename)
			lookup[normalized_name] = filename
		}

		var rows []map[string]interface{}
		err := json.Unmarshal([]byte(result.Response), &rows)
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

			filename, pres := lookup[name]
			if !pres {
				continue
			}

			download_info := &flows_proto.VFSDownloadInfo{}
			err := db.GetSubject(config_obj, filename, download_info)
			if err == nil {
				row["Download"] = download_info
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
	config_obj *config_proto.Config,
	components []string) (*flows_proto.VFSListResponse, error) {
	var rows []*FileInfoRow

	vfs_path := utils.JoinComponents(components, "/")
	items, err := file_store.GetFileStore(config_obj).
		ListDirectory(vfs_path)
	if err == nil {
		for _, item := range items {
			row := &FileInfoRow{
				Name:      item.Name(),
				Size:      item.Size(),
				Timestamp: item.ModTime().Format("2006-01-02 15:04:05"),
				FullPath:  utils.PathJoin(vfs_path, item.Name(), "/"),
			}

			if item.IsDir() {
				row.Mode = "dr-xr-xr-x"
			} else {
				row.Mode = "-r--r--r--"
				row.Download = &flows_proto.VFSDownloadInfo{
					VfsPath: row.FullPath,
					Size:    uint64(item.Size()),
					Mtime:   uint64(item.ModTime().UnixNano() / 1000),
				}
			}

			rows = append(rows, row)
		}
	}

	encoded_rows, err := json.MarshalIndent(rows, "", " ")
	if err != nil {
		return nil, err
	}

	result := &flows_proto.VFSListResponse{
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
func getVFSPathPrefix(components []string, client_id string) (prefix []string, ok bool) {
	if len(components) > 0 && components[0] == "artifacts" {
		return []string{"clients", client_id}, true
	}

	if client_id != "" && len(components) > 2 &&
		components[0] == "clients" && components[1] == client_id {
		return nil, true
	}

	return nil, false
}

func vfsListDirectory(
	config_obj *config_proto.Config,
	client_id string,
	vfs_path string) (*flows_proto.VFSListResponse, error) {

	components := utils.SplitComponents(vfs_path)
	if len(components) == 0 {
		return renderRootVFS(client_id), nil
	}

	prefix, ok := getVFSPathPrefix(components, client_id)
	if ok {
		return renderFileStore(config_obj, append(prefix, components...))
	}

	return renderDBVFS(config_obj, client_id, components)
}

// NOTE: We only support stat of DBFS style entries. This function is
// used to track when a directory changes in response to a refresh
// directory flow.
func vfsStatDirectory(
	config_obj *config_proto.Config,
	client_id string,
	vfs_path string) (*flows_proto.VFSListResponse, error) {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	vfs_urn := fmt.Sprintf("/clients/%v/vfs/%v", client_id, vfs_path)

	result := &flows_proto.VFSListResponse{}

	// Regardless of error we return success - if the file does
	// not exist yet then it will have no flow id associated with
	// it. This allows the gui to watch for the VFS directory to
	// appear for the first time.
	db.GetSubject(config_obj, vfs_urn, result)

	// Remove the actual response which might be large.
	result.Response = ""

	return result, nil
}

func vfsStatDownload(
	config_obj *config_proto.Config,
	client_id string,
	vfs_path string) (*flows_proto.VFSDownloadInfo, error) {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	components := utils.SplitComponents(vfs_path)
	vfs_components := append([]string{
		"clients", client_id, "vfs_files"}, components...)

	result := &flows_proto.VFSDownloadInfo{}

	// Regardless of error we return success - if the file does
	// not exist yet then it will have no flow id associated with
	// it. This allows the gui to watch for the VFS directory to
	// appear for the first time.
	vfs_urn := utils.JoinComponents(vfs_components, "/")
	err = db.GetSubject(config_obj, vfs_urn, result)
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
func GetClientPath(vfs_path string) (client_path string, accessor string) {
	components := utils.SplitComponents(vfs_path)

	if len(components) == 0 {
		return "", "file"
	}

	switch components[0] {
	case "file", "registry":
		return utils.JoinComponents(components[1:], "/"), components[0]

	case "ntfs":
		// With the ntfs accessor, first component is a device
		// and should not be preceeded with /
		return strings.Join(components[1:], "\\"), components[0]

	default:
		// This should not happen - try to get it using file accessor.
		return vfs_path, "file"
	}
}

func vfsRefreshDirectory(
	self *ApiServer,
	ctx context.Context,
	client_id string,
	vfs_path string,
	depth uint64) (*flows_proto.ArtifactCollectorResponse, error) {

	client_path, accessor := GetClientPath(vfs_path)
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
