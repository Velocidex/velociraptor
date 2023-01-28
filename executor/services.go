package executor

import (
	"context"
	"sync"

	"www.velocidex.com/golang/velociraptor/actions"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

func StartEventTableService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config,
	output_chan chan *crypto_proto.VeloMessage) error {

	logger := logging.GetLogger(config_obj, &logging.ClientComponent)
	logger.Info("<green>Starting</> event query service with version %v.",
		actions.GlobalEventTableVersion())

	actions.InitializeEventTable(ctx, config_obj, output_chan, wg)

	writeback, _ := config.GetWriteback(config_obj.Client)
	if writeback != nil && writeback.EventQueries != nil {
		actions.UpdateEventTable{}.Run(config_obj, ctx,
			output_chan, writeback.EventQueries)
	}

	logger.Info("<green>Starting</> event query service with version %v.",
		actions.GlobalEventTableVersion())

	return nil
}
