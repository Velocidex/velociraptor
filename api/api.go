package api

import (
	"fmt"
	context "golang.org/x/net/context"
	"google.golang.org/grpc"
	"net"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/logging"
)

type apiServer struct {
	config *config.Config
}

func (self *apiServer) LaunchFlow(
	ctx context.Context,
	in *api_proto.StartFlowRequest) (*api_proto.StartFlowResponse, error) {
	return &api_proto.StartFlowResponse{}, nil
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
		&apiServer{
			config: config_obj,
		},
	)

	logger := logging.NewLogger(config_obj)
	logger.Info("Launched API server on %v ", bind_addr)

	err = grpcServer.Serve(lis)
	if err != nil {
		return err
	}

	return nil
}
