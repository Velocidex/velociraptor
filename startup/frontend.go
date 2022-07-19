package startup

import (
	"context"

	"www.velocidex.com/golang/velociraptor/api"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/orgs"
)

// StartFrontendServices starts the binary as a frontend:
// 1.
func StartFrontendServices(
	ctx context.Context,
	config_obj *config_proto.Config) (*services.Service, error) {

	sm := services.NewServiceManager(ctx, config_obj)
	_, err := orgs.NewOrgManager(sm.Ctx, sm.Wg, config_obj)
	if err != nil {
		return sm, err
	}

	// Start the listening server
	server_builder, err := api.NewServerBuilder(sm.Ctx, config_obj, sm.Wg)
	if err != nil {
		return sm, err
	}

	// Start the gRPC API server on the master only.
	if services.IsMaster(config_obj) {
		err = server_builder.WithAPIServer(sm.Ctx, sm.Wg)
		if err != nil {
			return sm, err
		}
	}

	return sm, server_builder.StartServer(sm.Ctx, sm.Wg)
}
