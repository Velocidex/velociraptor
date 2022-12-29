/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2022 Rapid7 Inc.

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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

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
	case "auto", "file", "registry":
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

	var components string
	if len(vfs_components) > 0 {
		components = json.MustMarshalString(vfs_components[1:])
	}

	client_path, accessor := GetClientPath(vfs_components)
	request := MakeCollectorRequest(
		client_id, "System.VFS.ListDirectory",
		"Path", client_path,
		"Components", components,
		"Accessor", accessor,
		"Depth", fmt.Sprintf("%v", depth))

	// VFS navigation is high priority.
	request.Urgent = true

	result, err := self.CollectArtifact(ctx, request)
	return result, err
}

// Read the file listing table, but enrich the result with download
// info.
func (self *ApiServer) VFSListDirectoryFiles(
	ctx context.Context,
	in *api_proto.GetTableRequest) (*api_proto.GetTableResponse, error) {

	defer Instrument("VFSListDirectoryFiles")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.READ_RESULTS
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view the VFS.")
	}

	vfs_service, err := services.GetVFSService(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	stat, err := vfs_service.StatDirectory(
		org_config_obj, in.ClientId, in.VfsComponents)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	table_request := proto.Clone(in).(*api_proto.GetTableRequest)
	table_request.Artifact = stat.Artifact
	if table_request.Artifact == "" {
		table_request.Artifact = "System.VFS.ListDirectory"
	}

	// Transform the table into a subsection of the main table.
	table_request.StartIdx = stat.StartIdx
	table_request.EndIdx = stat.EndIdx

	// Get the table possibly applying any table transformations.
	result, err := getTable(ctx, org_config_obj, table_request)
	if err != nil {
		return nil, Status(self.verbose, err)
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

	db, err := datastore.GetDB(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	// Now enrich the files with download information.
	client_path_manager := paths.NewClientPathManager(in.ClientId)
	download_info_path := client_path_manager.VFSDownloadInfoPath(
		in.VfsComponents)
	downloaded_files, _ := db.ListChildren(org_config_obj, download_info_path)

	// Merge uploaded file info with the VFSListResponse.
	lookup := make(map[string]bool)
	for _, filename := range downloaded_files {
		lookup[filename.Base()] = true
	}

	for _, row := range result.Rows {
		if len(row.Cell) <= index_of_Name {
			continue
		}

		// Find the Name column entry in each cell.
		name := row.Cell[index_of_Name]

		// Insert a Download columns in the begining.
		row.Cell = append([]string{""}, row.Cell...)

		_, pres := lookup[name]
		if !pres {
			continue
		}

		// Make a copy for each path
		file_components := download_info_path.AddChild(name)
		download_info := &flows_proto.VFSDownloadInfo{}
		err := db.GetSubject(
			org_config_obj, file_components, download_info)
		if err == nil {
			// Support reading older
			// VFSDownloadInfo protobufs which
			// only contained the vfs_path and not
			// the components.
			if download_info.VfsPath != "" {
				download_info.Components = utils.SplitComponents(download_info.VfsPath)
			}

			row.Cell[0] = json.MustMarshalString(download_info)
		}
	}
	result.Columns = append([]string{"Download"}, result.Columns...)
	return result, nil
}
