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
	config_obj, err := config.LoadConfig("test_data/client.config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	ctx.Config = config_obj
	utils.Debug(ctx)

	manager, err := crypto.NewClientCryptoManager(
		config_obj, []byte(config_obj.Writeback.PrivateKey))
	if err != nil {
		t.Fatal(err)
	}

	exe, err := executor.NewClientExecutor(config_obj)
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

	_ = comm
	//	comm.Run()
}
