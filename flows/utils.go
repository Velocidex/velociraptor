package flows

import (
	"github.com/golang/protobuf/proto"
	config "www.velocidex.com/golang/velociraptor/config"
	datastore "www.velocidex.com/golang/velociraptor/datastore"
)

func QueueMessageForClient(
	config_obj *config.Config,
	client_id string,
	flow_id string,
	client_action_name string,
	message proto.Message,
	next_state uint64) error {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	return db.QueueMessageForClient(
		config_obj, client_id, flow_id,
		client_action_name, message, next_state)
}
