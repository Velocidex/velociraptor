// Client stubs for GRPC connections.
package grpc_client

import (
	"fmt"

	"google.golang.org/grpc"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
)

// TODO- Return a cluster dialer.
func GetChannel(config_obj *api_proto.Config) *grpc.ClientConn {
	address := fmt.Sprintf("%s:%d", config_obj.API.BindAddress, config_obj.API.BindPort)
	con, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		panic(fmt.Sprintf("Unable to connect to self: %v: %v", address, err))
	}
	return con
}
