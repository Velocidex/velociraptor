package startup

import (
	"context"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/encrypted_logs"
	"www.velocidex.com/golang/velociraptor/services/orgs"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

// StartClientServices starts the various services needed by the
// client.
func StartClientServices(
	ctx context.Context,
	config_obj *config_proto.Config,
	on_error func(ctx context.Context,
		config_obj *config_proto.Config)) (*services.Service, error) {

	scope := vql_subsystem.MakeScope()
	vql_subsystem.InstallUnimplemented(scope)

	// Create a suitable service plan.
	if config_obj.Services == nil {
		config_obj.Services = services.ClientServicesSpec()
	}

	// Wait for all services to properly start
	// before we begin the comms.
	sm := services.NewServiceManager(ctx, config_obj)

	// Start encrypted logs service if possible
	err := encrypted_logs.StartEncryptedLog(sm.Ctx, sm.Wg, sm.Config)
	if err != nil {
		return sm, err
	}

	// Start the nanny first so we are covered from here on.
	err = sm.Start(executor.StartNannyService)
	if err != nil {
		return sm, err
	}

	_, err = orgs.NewOrgManager(sm.Ctx, sm.Wg, sm.Config)
	if err != nil {
		return sm, err
	}

	return sm, err
}
