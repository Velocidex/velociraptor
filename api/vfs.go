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
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
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
