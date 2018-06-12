//
package flows

import (
	"errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/config"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	flow_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	//	utils "www.velocidex.com/golang/velociraptor/testing"
)

const (
	_                        = iota
	processClientInfo uint32 = 1
)

type Interogate struct{}

func (self *Interogate) Start(
	config_obj *config.Config,
	flow_runner_args *flow_proto.FlowRunnerArgs) (*string, error) {
	if flow_runner_args.ClientId == nil {
		return nil, errors.New("Client id not provided.")
	}
	client_id := *flow_runner_args.ClientId

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	flow_id := getNewFlowId(client_id)
	vql_request := &actions_proto.VQLCollectorArgs{}
	err = db.QueueMessageForClient(
		config_obj, client_id,
		flow_id,
		"VQLClientAction",
		vql_request, processClientInfo)
	if err != nil {
		return nil, err
	}

	return &flow_id, nil
}

func (self *Interogate) ProcessMessage(flow_obj *AFF4FlowObject,
	message *crypto_proto.GrrMessage) error {
	err := flow_obj.FailIfError(message)
	if err != nil {
		return err
	}
	return nil
}

func init() {
	impl := Interogate{}
	RegisterImplementation("Interogate", &impl)
}
