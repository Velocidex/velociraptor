package executor

import (
	"context"
	"sync"

	"www.velocidex.com/golang/velociraptor/actions"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/startup"
)

// Start services that are available on the client.
func StartServices(
	sm *services.Service,
	client_id string,
	exe *ClientExecutor) error {

	err := startup.StartupEssentialServices(sm)
	if err != nil {
		return err
	}

	// Now start client specific services.
	err = sm.Start(func(ctx context.Context,
		wg *sync.WaitGroup,
		config_obj *config_proto.Config) error {
		return StartEventTableService(ctx, wg, config_obj, exe)
	})
	if err != nil {
		return err
	}

	return sm.Start(StartNannyService)
}

func StartEventTableService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config,
	exe *ClientExecutor) error {

	logger := logging.GetLogger(config_obj, &logging.ClientComponent)
	logger.Info("<green>Starting</> event query service with version %v.",
		actions.GlobalEventTableVersion())

	responder := responder.NewResponder(
		config_obj, &crypto_proto.VeloMessage{
			SessionId: constants.MONITORING_WELL_KNOWN_FLOW,
		}, exe.Outbound)

	actions.InitializeEventTable(ctx, wg)

	writeback, _ := config.GetWriteback(config_obj.Client)
	if writeback != nil && writeback.EventQueries != nil {
		actions.UpdateEventTable{}.Run(config_obj, ctx,
			responder, writeback.EventQueries)
	}

	logger.Info("<green>Starting</> event query service with version %v.",
		actions.GlobalEventTableVersion())

	return nil
}
