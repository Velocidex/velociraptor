//
package flows

import (
	"github.com/golang/protobuf/proto"

	"errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	flow_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/responder"
	//	utils "www.velocidex.com/golang/velociraptor/testing"
)

const (
	_                        = iota
	processClientInfo uint64 = 1
)

type VInterrogate struct{}

func (self *VInterrogate) Start(
	config_obj *config.Config,
	flow_runner_args *flow_proto.FlowRunnerArgs) (*string, error) {
	if flow_runner_args.ClientId == "" {
		return nil, errors.New("Client id not provided.")
	}
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	flow_id := getNewFlowId(flow_runner_args.ClientId)
	queries := []*actions_proto.VQLRequest{
		&actions_proto.VQLRequest{
			"select Client_name, Client_build_time, Client_labels from config",
			"Client Info"},
		&actions_proto.VQLRequest{
			"select OS, Architecture, Platform, PlatformVersion, " +
				"KernelVersion, Fqdn from info()",
			"System Info"},
	}
	vql_request := &actions_proto.VQLCollectorArgs{
		Query: queries,
	}

	err = db.QueueMessageForClient(
		config_obj, flow_runner_args.ClientId,
		flow_id,
		"VQLClientAction",
		vql_request, processClientInfo)
	if err != nil {
		return nil, err
	}

	return &flow_id, nil
}

func (self *VInterrogate) ProcessMessage(
	config_obj *config.Config,
	flow_obj *AFF4FlowObject,
	message *crypto_proto.GrrMessage) error {
	err := flow_obj.FailIfError(message)
	if err != nil {
		return err
	}

	switch message.RequestId {
	case processClientInfo:
		client_info := &actions_proto.ClientInfo{}
		vql_response, ok := responder.ExtractGrrMessagePayload(
			message).(*actions_proto.VQLResponse)
		if ok {
			client_info.Info = append(client_info.Info, vql_response)
		}

		client_urn := "aff4:/" + flow_obj.runner_args.ClientId
		db, err := datastore.GetDB(config_obj)
		if err == nil {
			data := make(map[string][]byte)
			encoded_client_info, err := proto.Marshal(client_info)
			if err == nil {
				data[constants.CLIENT_VELOCIRAPTOR_INFO] = encoded_client_info
				db.SetSubjectData(config_obj, client_urn, data)
			}
		}

	}

	return nil
}

func init() {
	impl := VInterrogate{}
	RegisterImplementation("VInterrogate", &impl)
}
