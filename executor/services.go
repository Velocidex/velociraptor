package executor

import (
	"context"
	"fmt"

	"github.com/golang/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/actions"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

func StartServices(
	config_obj *config_proto.Config,
	client_id string,
	exe *ClientExecutor) {

	logger := logging.GetLogger(config_obj, &logging.ClientComponent)
	logger.Info("Starting event query service.")

	request := &crypto_proto.GrrMessage{
		SessionId: fmt.Sprintf("aff4:/clients/%v/flows/F.Monitoring",
			client_id),
		RequestId: 1,
		Name:      "UpdateEventTable",
		Source:    constants.FRONTEND_NAME,
		AuthState: crypto_proto.GrrMessage_AUTHENTICATED,
	}

	serialized_args, err := proto.Marshal(config_obj.Writeback.EventQueries)
	if err != nil {
		logger.Error(err)
		return
	}

	request.Args = serialized_args
	request.ArgsRdfName = "VQLEventTable"

	plugin := &actions.UpdateEventTable{}
	plugin.Run(config_obj, context.Background(), request, exe.Outbound)
}
