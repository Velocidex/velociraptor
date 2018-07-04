package flows

import (
	"errors"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"path"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config "www.velocidex.com/golang/velociraptor/config"
	constants "www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	datastore "www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	urns "www.velocidex.com/golang/velociraptor/urns"
)

const (
	_VFSListDirectory_state_1 uint64 = 1
)

type VFSListDirectory struct{}

func (self *VFSListDirectory) Start(
	config_obj *config.Config,
	flow_obj *AFF4FlowObject,
	args proto.Message) (*string, error) {

	vfs_args, ok := args.(*flows_proto.VFSListRequest)
	if !ok {
		return nil, errors.New("Expected args of type VQLCollectorArgs")
	}
	flow_obj.SetState(vfs_args)

	vql_collector_args := &actions_proto.VQLCollectorArgs{
		// Injecting the path in the environment avoids the
		// need to escape it within the query itself and it is
		// therefore more robust.
		Env: []*actions_proto.VQLEnv{
			{Key: "path", Value: path.Join("/", vfs_args.VfsPath)},
		},
		Query: []*actions_proto.VQLRequest{
			{
				Name: vfs_args.VfsPath,
				VQL: fmt.Sprintf(
					"SELECT IsDir, Name, Size, Mode from glob(" +
						"globs=path + '/*')"),
			},
		},
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
		vql_collector_args, _VFSListDirectory_state_1)
	if err != nil {
		return nil, err
	}

	return &flow_id, nil
}

func (self *VFSListDirectory) ProcessMessage(
	config_obj *config.Config,
	flow_obj *AFF4FlowObject,
	message *crypto_proto.GrrMessage) error {

	vfs_args := flow_obj.GetState().(*flows_proto.VFSListRequest)
	err := flow_obj.FailIfError(message)
	if err != nil {
		return err
	}

	if flow_obj.IsRequestComplete(message) {
		flow_obj.Complete()
		return nil
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}
	if message.ArgsRdfName != "VQLResponse" {
		return errors.New("Unexpected response type " + message.ArgsRdfName)
	}

	data := make(map[string][]byte)
	data[constants.VFS_FILE_LISTING] = message.Args

	urn := urns.BuildURN(
		flow_obj.RunnerArgs.ClientId, "vfs",
		vfs_args.VfsPath)

	fmt.Printf("Storing in urn %v", urn)

	err = db.SetSubjectData(config_obj, urn, 0, data)
	if err != nil {
		return err
	}

	return nil
}

func init() {
	impl := VFSListDirectory{}
	default_args, _ := ptypes.MarshalAny(&flows_proto.VFSListRequest{})
	desc := &flows_proto.FlowDescriptor{
		Name:         "VFSListDirectory",
		FriendlyName: "List VFS Directory",
		Category:     "Collectors",
		Doc:          "List a single directory in the client's filesystem.",
		ArgsType:     "VFSListRequest",
		DefaultArgs:  default_args,
	}
	RegisterImplementation(desc, &impl)
}
