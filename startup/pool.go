package startup

import (
	"context"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/http_comms"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/orgs"
)

// StartClientServices starts the various services needed by the
// client.
func StartPoolClientServices(
	sm *services.Service,
	config_obj *config_proto.Config,
	exe *executor.PoolClientExecutor) error {

	// Create a suitable service plan.
	if config_obj.Services == nil {
		config_obj.Services = services.ClientServicesSpec()
	}

	_, err := services.GetOrgManager()
	if err != nil {
		_, err = orgs.NewOrgManager(sm.Ctx, sm.Wg, config_obj)
		if err != nil {
			return err
		}
	}

	_, err = http_comms.StartHttpCommunicatorService(
		sm.Ctx, sm.Wg, config_obj, exe,
		func(ctx context.Context, config_obj *config_proto.Config) {})
	if err != nil {
		return err
	}

	/*
		err = executor.StartEventTableService(
			sm.Ctx, sm.Wg, config_obj, exe.Outbound)
	*/

	return nil
}
