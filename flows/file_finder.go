package flows

import (
	"errors"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config "www.velocidex.com/golang/velociraptor/config"
	datastore "www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
)

type FileFinder struct {
	*VQLCollector
}

func (self *FileFinder) Load(flow_obj *AFF4FlowObject) error {
	return nil
}

func (self *FileFinder) Save(flow_obj *AFF4FlowObject) error {
	return nil
}

func (self *FileFinder) Start(
	config_obj *config.Config,
	flow_obj *AFF4FlowObject,
	args proto.Message) (*string, error) {
	args, ok := args.(*flows_proto.FileFinderArgs)
	if !ok {
		return nil, errors.New("Expected args of type VInterrogateArgs")
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	flow_id := GetNewFlowIdForClient(flow_obj.RunnerArgs.ClientId)
	vql_request := &actions_proto.VQLCollectorArgs{}

	err = db.QueueMessageForClient(
		config_obj, flow_obj.RunnerArgs.ClientId,
		flow_id,
		"VQLClientAction",
		vql_request, processVQLResponses)
	if err != nil {
		return nil, err
	}

	return &flow_id, nil
}

func init() {
	impl := FileFinder{}
	default_args, _ := ptypes.MarshalAny(&flows_proto.FileFinderArgs{})
	desc := &flows_proto.FlowDescriptor{
		Name:         "FileFinder",
		FriendlyName: "File Finder",
		Category:     "Collectors",
		Doc:          "Interactively build a VQL query to search for files.",
		ArgsType:     "FileFinderArgs",
		DefaultArgs:  default_args,
	}
	RegisterImplementation(desc, &impl)

}
