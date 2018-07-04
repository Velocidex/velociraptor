package api

import (
	"fmt"
	"github.com/golang/protobuf/ptypes/empty"
	context "golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"net"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
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
	in *flows_proto.FlowRunnerArgs) (*api_proto.StartFlowResponse, error) {
	utils.Debug(in)
	result := &api_proto.StartFlowResponse{}
	flow_id, err := flows.StartFlow(self.config, in)
	if err != nil {
		return nil, err
	}
	result.FlowId = *flow_id
	result.RunnerArgs = in

	return result, nil
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
		api_client, err := GetApiClient(self.config, client_id, false)
		if err == nil {
			result.Items = append(result.Items, api_client)
		}
	}

	return result, nil
}

func (self *ApiServer) GetClient(
	ctx context.Context,
	in *api_proto.GetClientRequest) (*api_proto.ApiClient, error) {
	utils.Debug(in)
	api_client, err := GetApiClient(
		self.config,
		in.Query,
		true, // Detailed
	)
	if err != nil {
		return nil, err
	}

	return api_client, nil
}

func (self *ApiServer) DescribeTypes(
	ctx context.Context,
	in *empty.Empty) (*api_proto.Types, error) {
	return describeTypes(), nil
}

func (self *ApiServer) GetClientFlows(
	ctx context.Context,
	in *api_proto.ApiFlowRequest) (*api_proto.ApiFlowResponse, error) {
	utils.Debug(in)

	// HTTP HEAD requests against this method are used by the GUI
	// for auth checks.
	result := &api_proto.ApiFlowResponse{}
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		method := md.Get("METHOD")
		if len(method) > 0 && method[0] == "HEAD" {
			if IsUserApprovedForClient(self.config, &md, in.ClientId) {
				return result, nil
			}
			return nil, status.New(
				codes.PermissionDenied, "Not authorized").Err()
		}
	}

	result, err := getFlows(self.config, in.ClientId, in.Offset, in.Count)
	return result, err
}

func (self *ApiServer) GetClientApprovalForUser(
	ctx context.Context,
	in *api_proto.GetClientRequest) (*api_proto.ApprovalList, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		return getClientApprovalForUser(self.config, &md, in.Query), nil
	}
	return nil, status.New(
		codes.PermissionDenied, "Not authorized").Err()
}

func (self *ApiServer) GetUserUITraits(
	ctx context.Context,
	in *empty.Empty) (*api_proto.ApiGrrUser, error) {
	return NewDefaultUserObject(), nil
}

func (self *ApiServer) GetFlowDetails(
	ctx context.Context,
	in *api_proto.ApiFlowRequest) (*api_proto.ApiFlow, error) {
	utils.Debug(in)

	result, err := getFlowDetails(self.config, in.ClientId, in.FlowId)
	return result, err
}

func (self *ApiServer) GetFlowRequests(
	ctx context.Context,
	in *api_proto.ApiFlowRequest) (*api_proto.ApiFlowRequestDetails, error) {
	utils.Debug(in)
	result, err := getFlowRequests(self.config, in.ClientId, in.FlowId,
		in.Offset, in.Count)
	return result, err
}

func (self *ApiServer) GetFlowResults(
	ctx context.Context,
	in *api_proto.ApiFlowRequest) (*api_proto.ApiFlowResultDetails, error) {
	utils.Debug(in)
	result, err := getFlowResults(self.config, in.ClientId, in.FlowId,
		in.Offset, in.Count)
	return result, err
}

func (self *ApiServer) GetFlowDescriptors(
	ctx context.Context,
	in *empty.Empty) (*api_proto.FlowDescriptors, error) {
	result, err := getFlowDescriptors()
	return result, err
}

func (self *ApiServer) VFSListDirectory(
	ctx context.Context,
	in *flows_proto.VFSListRequest) (*actions_proto.VQLResponse, error) {
	utils.Debug(in)

	result, err := vfsListDirectory(
		self.config, in.ClientId, in.VfsPath)
	return result, err
}

func (self *ApiServer) VFSRefreshDirectory(
	ctx context.Context,
	in *api_proto.VFSRefreshDirectoryRequest) (
	*api_proto.StartFlowResponse, error) {
	utils.Debug(in)

	result, err := vfsRefreshDirectory(
		self, ctx, in.ClientId, in.VfsPath, in.Depth)
	return result, err
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
