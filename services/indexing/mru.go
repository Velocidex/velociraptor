package indexing

import (
	"time"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
)

func (self *Indexer) UpdateMRU(
	config_obj *config_proto.Config,
	user_name string, client_id string) error {
	path_manager := &paths.UserPathManager{Name: user_name}
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	item := &api_proto.ApiClient{
		ClientId:    client_id,
		FirstSeenAt: uint64(time.Now().Unix()),
	}

	return db.SetSubjectWithCompletion(
		config_obj, path_manager.MRUClient(client_id), item, nil)
}
