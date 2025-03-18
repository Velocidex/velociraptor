package startup

import (
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/orgs"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

// StartClientServices starts the various services needed by the
// client.
func StartPoolClientServices(
	sm *services.Service,
	config_obj *config_proto.Config) error {

	scope := vql_subsystem.MakeScope()
	scope.SetLogger(logging.NewPlainLogger(config_obj, &logging.ClientComponent))

	vql_subsystem.InstallUnimplemented(scope)

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

	return nil
}
