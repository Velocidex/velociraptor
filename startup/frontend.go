package startup

import (
	"context"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/api"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/orgs"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

// StartFrontendServices starts the binary as a frontend
func StartFrontendServices(
	ctx context.Context,
	config_obj *config_proto.Config) (*services.Service, error) {

	scope := vql_subsystem.MakeScope()
	vql_subsystem.InstallUnimplemented(scope)

	// Set the temp directory if needed
	executor.SetTempfile(config_obj)

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

	// Potentially restrict server functionality.
	if config_obj.Defaults != nil {
		if len(config_obj.Defaults.AllowedPlugins) > 0 {
			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Info("Restricting VQL plugins to set %v and functions to set %v\n",
				config_obj.Defaults.AllowedPlugins, config_obj.Defaults.AllowedFunctions)

			err = vql_subsystem.EnforceVQLAllowList(
				config_obj.Defaults.AllowedPlugins,
				config_obj.Defaults.AllowedFunctions)
			if err != nil {
				return sm, err
			}
		}

		if len(config_obj.Defaults.AllowedAccessors) > 0 {
			err = accessors.EnforceAccessorAllowList(
				config_obj.Defaults.AllowedAccessors)
			if err != nil {
				return sm, err
			}
		}
	}

	return sm, server_builder.StartServer(sm.Ctx, sm.Wg)
}
