/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"path"

	"github.com/Velocidex/yaml/v2"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	config "www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/http_comms"
	"www.velocidex.com/golang/velociraptor/server"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	pool_client_command = app.Command(
		"pool_client", "Run a pool client for load testing.")

	pool_client_number = pool_client_command.Flag(
		"number", "Total number of clients to run.").Int()

	pool_client_writeback_dir = pool_client_command.Flag(
		"writeback_dir", "The directory to store all writebacks.").Default(".").
		ExistingDir()
)

func doPoolClient() {
	client_config, err := DefaultConfigLoader.WithRequiredClient().LoadAndValidate()
	kingpin.FatalIfError(err, "Unable to load config file")

	server.IncreaseLimits(client_config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	number_of_clients := *pool_client_number
	if number_of_clients <= 0 {
		number_of_clients = 2
	}

	for i := 0; i < number_of_clients; i++ {
		client_config, err := DefaultConfigLoader.LoadAndValidate()
		kingpin.FatalIfError(err, "Unable to load config file")

		client_config.Client.WritebackLinux = path.Join(
			*pool_client_writeback_dir,
			fmt.Sprintf("pool_client.yaml.%d", i))

		client_config.Client.WritebackWindows = client_config.Client.WritebackLinux

		existing_writeback := &config_proto.Writeback{}
		data, err := ioutil.ReadFile(config.WritebackLocation(client_config))

		// Failing to read the file is not an error - the file may not
		// exist yet.
		if err == nil {
			err = yaml.Unmarshal(data, existing_writeback)
			kingpin.FatalIfError(err, "Unable to load config file")
		}

		// Merge the writeback with the config.
		client_config.Writeback = existing_writeback

		// Make sure the config is ok.
		err = crypto.VerifyConfig(client_config)
		if err != nil {
			kingpin.FatalIfError(err, "Invalid config")
		}

		manager, err := crypto.NewClientCryptoManager(
			client_config, []byte(client_config.Writeback.PrivateKey))
		kingpin.FatalIfError(err, "Unable to parse config file")

		exe, err := executor.NewClientExecutor(ctx, client_config)
		kingpin.FatalIfError(err, "Can not create executor.")

		comm, err := http_comms.NewHTTPCommunicator(
			client_config,
			manager,
			exe,
			client_config.Client.ServerUrls,
			utils.RealClock{},
		)
		kingpin.FatalIfError(err, "Can not create HTTPCommunicator.")

		// Run the client in the background.
		go comm.Run(ctx)
	}

	// Block forever.
	<-ctx.Done()
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case "pool_client":
			doPoolClient()
		default:
			return false
		}
		return true
	})
}
