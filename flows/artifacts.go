package flows

import (
	"errors"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	config "www.velocidex.com/golang/velociraptor/config"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
)

type ArtifactCollector struct {
	*VQLCollector
}

func (self *ArtifactCollector) New() Flow {
	return &ArtifactCollector{&VQLCollector{}}
}

func (self *ArtifactCollector) Start(
	config_obj *config.Config,
	flow_obj *AFF4FlowObject,
	args proto.Message) error {
	collector_args, ok := args.(*flows_proto.ArtifactCollectorArgs)
	if !ok {
		return errors.New("Expected args of type ArtifactCollectorArgs")
	}

	repository, err := artifacts.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	vql_collector_args := &actions_proto.VQLCollectorArgs{}
	for _, name := range collector_args.Artifacts.Names {
		artifact, pres := repository.Get(name)
		if !pres {
			return errors.New("Unknown artifact " + name)
		}

		err := artifacts.Compile(artifact, vql_collector_args)
		if err != nil {
			return err
		}
	}

	// Add any artifact dependencies.
	repository.PopulateArtifactsVQLCollectorArgs(vql_collector_args)
	return QueueMessageForClient(
		config_obj, flow_obj,
		"VQLClientAction",
		vql_collector_args, processVQLResponses)
}

func init() {
	impl := ArtifactCollector{&VQLCollector{}}
	default_args, _ := ptypes.MarshalAny(&flows_proto.ArtifactCollectorArgs{})
	desc := &flows_proto.FlowDescriptor{
		Name:         "ArtifactCollector",
		FriendlyName: "Artifact Collector",
		Category:     "Collectors",
		Doc:          "Collects multiple artifacts at once.",
		ArgsType:     "ArtifactCollectorArgs",
		DefaultArgs:  default_args,
	}

	RegisterImplementation(desc, &impl)
}
