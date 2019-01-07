package flows

import (
	"context"

	"github.com/golang/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	datastore "www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/grpc_client"
)

func QueueMessageForClient(
	config_obj *api_proto.Config,
	flow_obj *AFF4FlowObject,
	client_action_name string,
	message proto.Message,
	next_state uint64) error {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	return db.QueueMessageForClient(
		config_obj, flow_obj.RunnerArgs.ClientId,
		flow_obj.Urn,
		client_action_name, message, next_state)
}

func QueueAndNotifyClient(
	config_obj *api_proto.Config,
	client_id string,
	flow_urn string,
	client_action_name string,
	message proto.Message,
	next_state uint64) error {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	err = db.QueueMessageForClient(
		config_obj, client_id,
		flow_urn, client_action_name,
		message, next_state)
	if err != nil {
		return err
	}

	channel := grpc_client.GetChannel(config_obj)
	defer channel.Close()

	client := api_proto.NewAPIClient(channel)
	_, err = client.NotifyClients(context.Background(), &api_proto.NotificationRequest{
		ClientId: client_id,
	})

	return err
}
