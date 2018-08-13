package flows

import (
	"errors"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"path"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	config "www.velocidex.com/golang/velociraptor/config"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/responder"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

const (
	_                          = iota
	processVQLResponses uint64 = iota
)

type VQLCollector struct {
	*BaseFlow
}

func (self *VQLCollector) Start(
	config_obj *config.Config,
	flow_obj *AFF4FlowObject,
	args proto.Message) error {
	vql_collector_args, ok := args.(*actions_proto.VQLCollectorArgs)
	if !ok {
		return errors.New("Expected args of type VQLCollectorArgs")
	}

	// Add any required artifacts to the request.
	repository, err := artifacts.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}
	repository.PopulateVQLCollectorArgs(vql_collector_args)
	err = QueueMessageForClient(
		config_obj, flow_obj,
		"VQLClientAction",
		vql_collector_args, processVQLResponses)
	if err != nil {
		return err
	}

	return nil
}

func (self *VQLCollector) ProcessMessage(
	config_obj *config.Config,
	flow_obj *AFF4FlowObject,
	message *crypto_proto.GrrMessage) error {
	err := flow_obj.FailIfError(config_obj, message)
	if err != nil {
		return err
	}

	switch message.RequestId {
	case processVQLResponses:
		if flow_obj.IsRequestComplete(message) {
			return flow_obj.Complete(config_obj)
		}

		err = StoreResultInFlow(config_obj, flow_obj, message)
		if err != nil {
			return err
		}

		// Receive any file upload the client sent.
	case vql_subsystem.TransferWellKnownFlowId:
		return appendDataToFile(
			config_obj, flow_obj,
			path.Join(flow_obj.RunnerArgs.ClientId,
				path.Base(message.SessionId)),
			message)
	}
	return nil
}

func appendDataToFile(
	config_obj *config.Config,
	flow_obj *AFF4FlowObject,
	base_urn string,
	message *crypto_proto.GrrMessage) error {
	payload := responder.ExtractGrrMessagePayload(message)
	if payload == nil {
		return nil
	}
	file_buffer, ok := payload.(*actions_proto.FileBuffer)
	if !ok {
		return nil
	}
	file_store_factory := file_store.GetFileStore(config_obj)
	file_path := path.Join(base_urn, file_buffer.Pathspec.Path)
	fd, err := file_store_factory.WriteFile(file_path)
	if err != nil {
		return err
	}
	defer fd.Close()

	fd.Seek(int64(file_buffer.Offset), 0)
	fd.Write(file_buffer.Data)

	// Keep track of all the files we uploaded.
	if file_buffer.Offset == 0 {
		flow_obj.FlowContext.UploadedFiles = append(
			flow_obj.FlowContext.UploadedFiles,
			file_path)
		flow_obj.dirty = true
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
