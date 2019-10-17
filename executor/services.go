package executor

import (
	"context"

	"www.velocidex.com/golang/velociraptor/actions"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/responder"
)

func StartServices(
	config_obj *config_proto.Config,
	client_id string,
	exe *ClientExecutor) {

	logger := logging.GetLogger(config_obj, &logging.ClientComponent)
	logger.Info("Starting event query service.")

	responder := responder.NewResponder(config_obj, &crypto_proto.GrrMessage{}, nil)
	if config_obj.Writeback.EventQueries != nil {
		actions.UpdateEventTable{}.Run(config_obj, context.Background(),
			responder, config_obj.Writeback.EventQueries)
	}
}
