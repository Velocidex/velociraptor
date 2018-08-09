package main

import (
	"gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/context"
	"www.velocidex.com/golang/velociraptor/crypto"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/http_comms"
)

var (
	// Run the client.
	client = app.Command("client", "Run the velociraptor client")
)

func RunClient(config_path *string) {
	ctx := context.Background()

	if config_path == nil || *config_path == "" {
		kingpin.Fatalf("config file must be provided.")
	}

	config_obj, err := config.LoadClientConfig(*config_path)
	if err != nil {
		kingpin.FatalIfError(err, "Unable to load config file")
	}
	ctx.Config = config_obj

	// Make sure the config is ok.
	err = crypto.VerifyConfig(ctx.Config)
	if err != nil {
		kingpin.FatalIfError(err, "Invalid config")
	}

	manager, err := crypto.NewClientCryptoManager(
		config_obj, []byte(config_obj.Writeback.PrivateKey))
	if err != nil {
		kingpin.FatalIfError(err, "Unable to parse config file")
	}

	exe, err := executor.NewClientExecutor(config_obj)
	if err != nil {
		kingpin.FatalIfError(err, "Can not create executor.")
	}

	comm, err := http_comms.NewHTTPCommunicator(
		ctx,
		manager,
		exe,
		config_obj.Client.ServerUrls,
	)
	if err != nil {
		kingpin.FatalIfError(err, "Can not create HTTPCommunicator.")
	}

	comm.Run()
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		if command == client.FullCommand() {
			RunClient(config_path)
			return true
		}
		return false
	})
}
