package executor

import (
	"context"
	"errors"
	"sync"

	"www.velocidex.com/golang/velociraptor/actions"
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
	if config_obj.Writeback == nil {
		return errors.New("Client writeback not configured")
	}

	logger := logging.GetLogger(config_obj, &logging.ClientComponent)
	logger.Info("<green>Starting</> event query service with version %v.",
		actions.GlobalEventTableVersion())

	responder := responder.NewResponder(
		config_obj, &crypto_proto.GrrMessage{
			SessionId: constants.MONITORING_WELL_KNOWN_FLOW,
		}, exe.Outbound)
	if config_obj.Writeback.EventQueries != nil {
		actions.UpdateEventTable{}.Run(config_obj, ctx,
			responder, config_obj.Writeback.EventQueries)
	}

	logger.Info("<green>Starting</> event query service with version %v.",
		actions.GlobalEventTableVersion())

	return nil
}
