package api

import (
	"github.com/golang/protobuf/proto"
	"path"
	"strings"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config "www.velocidex.com/golang/velociraptor/config"
	constants "www.velocidex.com/golang/velociraptor/constants"
	datastore "www.velocidex.com/golang/velociraptor/datastore"
	utils "www.velocidex.com/golang/velociraptor/testing"
	urns "www.velocidex.com/golang/velociraptor/urns"
)

func vfsListDirectory(
	config_obj *config.Config,
	client_id string,
	vfs_path string) (*actions_proto.VQLResponse, error) {
	vfs_path = path.Join("/", vfs_path)

	result := &actions_proto.VQLResponse{}

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
	utils.Debug(vfs_urn)

	data, err := db.GetSubjectData(config_obj, vfs_urn, 0, 10)
	if err != nil {
		return nil, err
	}

	serialized_response, pres := data[constants.VFS_FILE_LISTING]
	if pres {
		err = proto.Unmarshal(serialized_response, result)
		if err != nil {
			return nil, err
		}
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
