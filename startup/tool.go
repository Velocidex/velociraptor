package startup

import (
	"context"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/orgs"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

// Start minimal services for tools.
func StartToolServices(
	ctx context.Context,
	config_obj *config_proto.Config) (*services.Service, error) {

	scope := vql_subsystem.MakeScope()
	vql_subsystem.InstallUnimplemented(scope)

	sm := services.NewServiceManager(ctx, config_obj)

	err := MaybeEnforceAllowLists(config_obj)
	if err != nil {
		return sm, err
	}

	_, err = orgs.NewOrgManager(sm.Ctx, sm.Wg, config_obj)
	if err != nil {
		return sm, err
	}

	return sm, nil
}
