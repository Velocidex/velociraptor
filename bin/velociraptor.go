package main

import (
	"gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/http_comms"
	"www.velocidex.com/golang/velociraptor/context"
	"www.velocidex.com/golang/velociraptor/crypto"
	"www.velocidex.com/golang/velociraptor/executor"
)

var (
	config_path = kingpin.Arg("config", "The client's config file.").Required().String()
)


func main() {
	kingpin.Parse()

	ctx := context.Background()
	config, err := config.LoadConfig(*config_path)
	if err != nil {
		kingpin.FatalIfError(err, "Unable to load config file")
	}
	ctx.Config = *config
	manager, err := crypto.NewClientCryptoManager(
		&ctx, []byte(config.Client_private_key))
	if err != nil {
		kingpin.FatalIfError(err, "Unable to parse config file")
	}

	exe, err := executor.NewClientExecutor(&ctx)
	if err != nil {
		kingpin.FatalIfError(err, "Can not create executor.")
	}

	comm, err := http_comms.NewHTTPCommunicator(
		ctx,
		manager,
		exe,
		config.Client_server_urls,
	)
	if err != nil {
		kingpin.FatalIfError(err, "Can not create HTTPCommunicator.")
	}

	comm.Run()

}
