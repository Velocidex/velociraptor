package api

import (
	"fmt"
	"github.com/golang/protobuf/descriptor"
	"github.com/golang/protobuf/proto"
	"strings"
	//	descriptor_proto "github.com/golang/protobuf/protoc-gen-go/descriptor"
	"github.com/golang/protobuf/ptypes/empty"
	context "golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"net"
	"reflect"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/flows"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	utils "www.velocidex.com/golang/velociraptor/testing"
)

type ApiServer struct {
	config *config.Config
}

func (self *ApiServer) LaunchFlow(
	ctx context.Context,
	in *api_proto.StartFlowRequest) (*api_proto.StartFlowResponse, error) {
	utils.Debug(in)
	var flow_name string
	var args proto.Message

	// This should be a oneof by currently grpc-gateway lacks
	// support for oneof.
	if in.Interrogate != nil {
		flow_name = "VInterrogate"
		args = in.Interrogate

	}

	flow_runner_args := &flows_proto.FlowRunnerArgs{
		ClientId: in.ClientId,
		FlowName: flow_name,
	}

	flow_id, err := flows.StartFlow(self.config, flow_runner_args, args)
	if err != nil {
		return nil, err
	}

	return &api_proto.StartFlowResponse{
		FlowId: *flow_id,
	}, nil
}

func (self *ApiServer) ListClients(
	ctx context.Context,
	in *api_proto.SearchClientsRequest) (*api_proto.SearchClientsResponse, error) {
	utils.Debug(in)

	db, err := datastore.GetDB(self.config)
	if err != nil {
		return nil, err
	}

	limit := uint64(50)
	if in.Limit > 0 {
		limit = in.Limit
	}

	result := &api_proto.SearchClientsResponse{}
	for _, client_id := range db.SearchClients(
		self.config, constants.CLIENT_INDEX_URN,
		in.Query, in.Offset, limit) {
		api_client, err := GetApiClient(self.config, client_id)
		if err == nil {
			result.Items = append(result.Items, api_client)
		}
	}

	return result, nil
}

func add_type(type_name string, result *api_proto.Types, seen map[string]bool) {
	type_name = strings.TrimPrefix(type_name, ".")
	message_type := proto.MessageType(type_name)
	if message_type == nil {
		return
	}

	new_message := reflect.New(message_type.Elem()).Interface().(descriptor.Message)
	_, md := descriptor.ForMessage(new_message)
	type_desc := &api_proto.TypeDescriptor{
		TypeName:    type_name,
		Descriptor_: md,
	}
	result.Descriptors = append(result.Descriptors, type_desc)
	seen[type_name] = true

	for _, field := range md.Field {
		if field.TypeName != nil {
			add_type(*field.TypeName, result, seen)
		}
	}
}

func (self *ApiServer) DescribeLaunchFlow(
	ctx context.Context,
	in *empty.Empty) (*api_proto.Types, error) {

	result := &api_proto.Types{}
	seen := make(map[string]bool)
	add_type("proto.StartFlowRequest", result, seen)
	return result, nil
}

func StartServer(config_obj *config.Config) error {
	bind_addr := fmt.Sprintf("%s:%d", *config_obj.API_bind_address,
		*config_obj.API_bind_port)

	lis, err := net.Listen("tcp", bind_addr)
	if err != nil {
		return err
	}

	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)

	api_proto.RegisterAPIServer(
		grpcServer,
		&ApiServer{
			config: config_obj,
		},
	)
	// Register reflection service.
	reflection.Register(grpcServer)

	logger := logging.NewLogger(config_obj)
	logger.Info("Launched API server on %v ", bind_addr)

	err = grpcServer.Serve(lis)
	if err != nil {
		return err
	}

	return nil
}
