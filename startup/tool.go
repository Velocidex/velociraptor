package startup

import (
	"context"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/executor/throttler"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/orgs"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/parsers/journald"
)

// Start minimal services for tools.
func StartToolServices(
	ctx context.Context,
	config_obj *config_proto.Config) (*services.Service, error) {

	scope := vql_subsystem.MakeScope()
	scope.SetLogger(logging.NewPlainLogger(config_obj, &logging.ToolComponent))

	vql_subsystem.InstallUnimplemented(scope)
	journald.StartGlobalJournaldService(ctx, config_obj)

	sm := services.NewServiceManager(ctx, config_obj)

	err := MaybeEnforceAllowLists(config_obj)
	if err != nil {
		return sm, err
	}

	// Start throttling service
	err = sm.Start(throttler.StartStatsCollectorService)
	if err != nil {
		return sm, err
	}

	_, err = orgs.NewOrgManager(sm.Ctx, sm.Wg, config_obj)
	if err != nil {
		return sm, err
	}

	return sm, nil
}
