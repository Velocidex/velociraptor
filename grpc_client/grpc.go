// Client stubs for GRPC connections.
package grpc_client

import (
	"fmt"

	"google.golang.org/grpc"
	"www.velocidex.com/golang/velociraptor/config"
)

// TODO- Return a cluster dialer.
func GetChannel(config_obj *config.Config) *grpc.ClientConn {
	address := fmt.Sprintf("%s:%d", config_obj.API.BindAddress, config_obj.API.BindPort)
	con, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		panic(fmt.Sprintf("Unable to connect to self: %v: %v", address, err))
	}
	return con
}
