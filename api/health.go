package api

import (
	"context"

	"www.velocidex.com/golang/velociraptor/api/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
)

func (self *ApiServer) Check(
	ctx context.Context,
	in *api_proto.HealthCheckRequest) (*api_proto.HealthCheckResponse, error) {

	return &proto.HealthCheckResponse{
		Status: api_proto.HealthCheckResponse_SERVING,
	}, nil
}
