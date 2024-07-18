package http_comms

import (
	"context"
	"fmt"
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_client "www.velocidex.com/golang/velociraptor/crypto/client"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/services/writeback"
	"www.velocidex.com/golang/velociraptor/utils"
)

func StartHttpCommunicatorService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config,
	exe executor.Executor,
	on_error func(ctx context.Context, config_obj *config_proto.Config)) (
	*HTTPCommunicator, error) {

	if config_obj.Client == nil {
		return nil, nil
	}

	writeback_service := writeback.GetWritebackService()
	writeback, err := writeback_service.GetWriteback(config_obj)
	if err != nil {
		return nil, err
	}

	crypto_manager, err := crypto_client.NewClientCryptoManager(ctx,
		config_obj, []byte(writeback.PrivateKey))
	if err != nil {
		return nil, err
	}

	// Now start the communicator so we can talk with the server.
	comm, err := NewHTTPCommunicator(
		ctx,
		config_obj,
		crypto_manager,
		exe,
		config_obj.Client.ServerUrls,
		func() { on_error(ctx, config_obj) },
		utils.RealClock{},
	)
	if err != nil {
		return nil, fmt.Errorf("Can not create HTTPCommunicator: %w", err)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		comm.Run(ctx, wg)
	}()

	return comm, nil
}
