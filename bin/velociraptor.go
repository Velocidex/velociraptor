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
	config_obj := config.GetDefaultConfig()

	// Can provide the config file on the command line OR embedded in the binary.
	if config_path != nil && *config_path != "" {
		err := config.LoadConfig(*config_path, config_obj)
		if err != nil {
			kingpin.FatalIfError(err, "Unable to load config file")
		}

	} else {
		// Packed binaries contain their config embedded in the
		// binary.
		config_string, err := ExtractEmbeddedConfig()
		if err != nil {
			kingpin.FatalIfError(err, "Unable to load embedded config file")
		}

		err = config.ParseConfigFromString(config_string, config_obj)
		if err != nil {
			kingpin.FatalIfError(err, "Unable to load config file")
		}
	}

	// Allow the embedded config to specify a writeback
	// location. We load that location in addition to the
	// configuration we were provided.
	if config_obj.Writeback != "" {
		err := config.LoadConfig(config_obj.Writeback, config_obj)
		if err != nil {
			kingpin.Errorf("Unable to load writeback file: %v", err)
		}
	}
	ctx.Config = config_obj

	// Make sure the config is ok.
	err := crypto.VerifyConfig(ctx.Config)
	if err != nil {
		kingpin.FatalIfError(err, "Invalid config")
	}

	manager, err := crypto.NewClientCryptoManager(
		config_obj, []byte(config_obj.Client.PrivateKey))
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
