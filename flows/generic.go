package flows

import (
	"errors"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"path"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config "www.velocidex.com/golang/velociraptor/config"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	datastore "www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/responder"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

const (
	_                          = iota
	processVQLResponses uint64 = iota
)

type VQLCollector struct{}

func (self *VQLCollector) Start(
	config_obj *config.Config,
	flow_obj *AFF4FlowObject,
	args proto.Message) (*string, error) {
	vql_collector_args, ok := args.(*actions_proto.VQLCollectorArgs)
	if !ok {
		return nil, errors.New("Expected args of type VQLCollectorArgs")
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	flow_id := GetNewFlowIdForClient(flow_obj.RunnerArgs.ClientId)
	err = db.QueueMessageForClient(
		config_obj, flow_obj.RunnerArgs.ClientId,
		flow_id,
		"VQLClientAction",
		vql_collector_args, processVQLResponses)
	if err != nil {
		return nil, err
	}

	return &flow_id, nil
}

func (self *VQLCollector) ProcessMessage(
	config_obj *config.Config,
	flow_obj *AFF4FlowObject,
	message *crypto_proto.GrrMessage) error {

	switch message.RequestId {
	case processVQLResponses:
		err := flow_obj.FailIfError(message)
		if err != nil {
			return err
		}

		if flow_obj.IsRequestComplete(message) {
			flow_obj.Complete()
			return nil
		}

		err = StoreResultInFlow(config_obj, flow_obj, message)
		if err != nil {
			return err
		}

		// Receive any file upload the client sent.
	case vql_subsystem.TransferWellKnownFlowId:
		payload := responder.ExtractGrrMessagePayload(message)
		if payload != nil {
			file_buffer, ok := payload.(*actions_proto.FileBuffer)
			if ok {
				file_store_factory := file_store.GetFileStore(
					config_obj)
				file_path := path.Join(
					flow_obj.RunnerArgs.ClientId,
					path.Base(message.SessionId),
					file_buffer.Pathspec.Path)
				fd, err := file_store_factory.WriteFile(
					file_path)
				if err != nil {
					return err
				}

				defer fd.Close()

				fd.Seek(int64(file_buffer.Offset), 0)
				fd.Write(file_buffer.Data)
			}
		}
	}
	return nil
}

func init() {
	impl := VQLCollector{}
	default_args, _ := ptypes.MarshalAny(&actions_proto.VQLCollectorArgs{})
	desc := &flows_proto.FlowDescriptor{
		Name:         "VQLCollector",
		FriendlyName: "VQL Collector",
		Category:     "Collectors",
		Doc:          "Issues VQL queries to the Velociraptor client and collects the results.",
		ArgsType:     "VQLCollectorArgs",
		DefaultArgs:  default_args,
	}

	RegisterImplementation(desc, &impl)
}
