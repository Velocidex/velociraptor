package startup

import (
	"context"
	"fmt"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/http_comms"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/orgs"
)

// StartClientServices starts the various services needed by the
// client.
func StartClientServices(
	ctx context.Context,
	config_obj *config_proto.Config,
	exe *executor.ClientExecutor,
	on_error func(ctx context.Context,
		config_obj *config_proto.Config)) (*services.Service, error) {

	// Create a suitable service plan.
	if config_obj.Frontend == nil {
		config_obj.Frontend = &config_proto.FrontendConfig{}
	}

	if config_obj.Frontend.ServerServices == nil {
		config_obj.Frontend.ServerServices = services.ClientServicesSpec()
	}

	fmt.Printf("StartClientServices for nonce %v\n\n", config_obj.Client.Nonce)

	// Wait for all services to properly start
	// before we begin the comms.
	sm := services.NewServiceManager(ctx, config_obj)

	// Start the nanny first so we are covered from here on.
	err := sm.Start(executor.StartNannyService)
	if err != nil {
		return sm, err
	}

	_, err = orgs.NewOrgManager(sm.Ctx, sm.Wg, sm.Config)
	if err != nil {
		return sm, err
	}

	err = http_comms.StartHttpCommunicatorService(
		ctx, sm.Wg, config_obj, exe, on_error)
	if err != nil {
		return sm, err
	}

	err = executor.StartEventTableService(
		ctx, sm.Wg, config_obj, exe.Outbound)

	return sm, err
}
