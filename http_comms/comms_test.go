package http_comms

import (
	"testing"

	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/context"
	"www.velocidex.com/golang/velociraptor/crypto"
	"www.velocidex.com/golang/velociraptor/executor"
	utils "www.velocidex.com/golang/velociraptor/testing"
)

func TestHTTPComms(t *testing.T) {
	ctx := context.Background()
	config, err := config.LoadConfig("test_data/client.config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	ctx.Config = *config
	utils.Debug(ctx)

	manager, err := crypto.NewClientCryptoManager(
		&ctx, []byte(config.Client_private_key))
	if err != nil {
		t.Fatal(err)
	}

	exe, err := executor.NewClientExecutor(&ctx)
	if err != nil {
		t.Fatal(err)
	}

	comm, err := NewHTTPCommunicator(
		ctx,
		manager,
		exe,
		[]string{
			"http://localhost:8080/",
		})
	if err != nil {
		t.Fatal(err)
	}

	comm.Run()
}
