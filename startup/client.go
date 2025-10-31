package startup

import (
	"context"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/executor/throttler"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/encrypted_logs"
	"www.velocidex.com/golang/velociraptor/services/orgs"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/debug"
	"www.velocidex.com/golang/velociraptor/vql/networking"
	"www.velocidex.com/golang/velociraptor/vql/parsers/journald"
)

// StartClientServices starts the various services needed by the
// client.
func StartClientServices(
	ctx context.Context,
	config_obj *config_proto.Config,
	on_error func(ctx context.Context,
		config_obj *config_proto.Config)) (*services.Service, error) {

	// Create a suitable service plan.
	if config_obj.Services == nil {
		config_obj.Services = services.ClientServicesSpec()
	}

	// Wait for all services to properly start
	// before we begin the comms.
	sm := services.NewServiceManager(ctx, config_obj)

	err := MaybeEnforceAllowLists(config_obj)
	if err != nil {
		return sm, err
	}

	scope := vql_subsystem.MakeScope()
	scope.SetLogger(logging.NewPlainLogger(config_obj, &logging.ClientComponent))

	vql_subsystem.InstallUnimplemented(scope)

	// Maybe add various debug plugins if we are in debug mode.
	debug.AddDebugPlugins(config_obj)

	// Start the journald watcher service if needed.
	journald.StartGlobalJournaldService(ctx, config_obj)

	// Start encrypted logs service if possible
	err = encrypted_logs.StartEncryptedLog(sm.Ctx, sm.Wg, sm.Config)
	if err != nil {
		return sm, err
	}

	err = networking.MaybeInstallDNSCache(sm.Ctx, sm.Wg, sm.Config)
	if err != nil {
		return sm, err
	}

	// Start the nanny first so we are covered from here on.
	err = sm.Start(executor.StartNannyService)
	if err != nil {
		return sm, err
	}

	// Start throttling service
	err = sm.Start(throttler.StartStatsCollectorService)
	if err != nil {
		return sm, err
	}

	_, err = orgs.NewOrgManager(sm.Ctx, sm.Wg, sm.Config)
	if err != nil {
		return sm, err
	}

	return sm, err
}
