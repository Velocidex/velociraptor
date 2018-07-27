package api

import (
	"github.com/golang/protobuf/ptypes"
	context "golang.org/x/net/context"
	"path"
	"strings"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config "www.velocidex.com/golang/velociraptor/config"
	datastore "www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	urns "www.velocidex.com/golang/velociraptor/urns"
)

func vfsListDirectory(
	config_obj *config.Config,
	client_id string,
	vfs_path string) (*actions_proto.VQLResponse, error) {
	vfs_path = path.Join("/", vfs_path)

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	virtual_dir_response, pres := getVirtualDirectory(vfs_path)
	if pres {
		return virtual_dir_response, nil
	}

	vfs_path = strings.TrimPrefix(vfs_path, "/fs")
	vfs_urn := urns.BuildURN(client_id, "vfs", vfs_path)

	result := &actions_proto.VQLResponse{}
	err = db.GetSubject(config_obj, vfs_urn, result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func getVirtualDirectory(vfs_path string) (*actions_proto.VQLResponse, bool) {
	if vfs_path == "" || vfs_path == "/" {
		return &actions_proto.VQLResponse{
			Response: "[{\"IsDir\": true, \"Name\": \"fs\"}]",
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

	// Trim the /fs from the VFS path to get the real path.
	vfs_path = path.Join("/", vfs_path)
	vfs_path = strings.TrimPrefix(vfs_path, "/fs")

	args := &flows_proto.FlowRunnerArgs{
		ClientId: client_id,
		FlowName: "VFSListDirectory",
	}

	flow_args := &flows_proto.VFSListRequest{
		VfsPath: vfs_path,
	}
	any_msg, err := ptypes.MarshalAny(flow_args)
	if err != nil {
		return nil, err
	}

	args.Args = any_msg

	result, err := self.LaunchFlow(ctx, args)
	return result, err
}
