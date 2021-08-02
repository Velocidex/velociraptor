package search

import (
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
)

func UpdateMRU(
	config_obj *config_proto.Config,
	user_name string, client_id string) error {
	path_manager := &paths.UserPathManager{user_name}
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	item := &api_proto.ApiClient{ClientId: client_id}
	return db.SetSubject(
		config_obj, path_manager.MRUClient(client_id), item)
}
