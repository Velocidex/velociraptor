package startup

import (
	"context"

	"www.velocidex.com/golang/velociraptor/api"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/orgs"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

// StartFrontendServices starts the binary as a frontend
func StartFrontendServices(
	ctx context.Context,
	config_obj *config_proto.Config) (*services.Service, error) {

	// Set the temp directory if needed
	tempfile.SetTempfile(config_obj)

	sm := services.NewServiceManager(ctx, config_obj)

	// Potentially restrict server functionality.
	err := MaybeEnforceAllowLists(config_obj)
	if err != nil {
		return sm, err
	}

	scope := vql_subsystem.MakeScope()
	scope.SetLogger(logging.NewPlainLogger(config_obj, &logging.FrontendComponent))

	vql_subsystem.InstallUnimplemented(scope)

	_, err = orgs.NewOrgManager(sm.Ctx, sm.Wg, config_obj)
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
