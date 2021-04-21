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
	"sync"

	"github.com/Velocidex/yaml/v2"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	config "www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_client "www.velocidex.com/golang/velociraptor/crypto/client"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/http_comms"
	"www.velocidex.com/golang/velociraptor/json"
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

	pool_client_concurrency = pool_client_command.Flag(
		"concurrency", "How many real queries to run.").Default("10").Int()
)

type counter struct {
	i  int
	mu sync.Mutex
}

func (self *counter) Inc() {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.i++

	if self.i%100 == 0 {
		fmt.Printf("Starting %v clients\n", self.i)
	}

}

func doPoolClient() {
	number_of_clients := *pool_client_number
	if number_of_clients <= 0 {
		number_of_clients = 2
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client_config, err := DefaultConfigLoader.
		WithRequiredClient().
		WithVerbose(*verbose_flag).
		LoadAndValidate()
	kingpin.FatalIfError(err, "Unable to load config file")

	sm, err := startEssentialServices(client_config)
	kingpin.FatalIfError(err, "Starting services.")
	defer sm.Close()

	server.IncreaseLimits(client_config)

	// Make a copy of all the configs for each client.
	configs := make([]*config_proto.Config, 0, number_of_clients)
	serialized, _ := json.Marshal(client_config)

	for i := 0; i < number_of_clients; i++ {
		client_config := &config_proto.Config{}
		err := json.Unmarshal(serialized, &client_config)
		kingpin.FatalIfError(err, "Copying configs.")
		configs = append(configs, client_config)
	}

	c := counter{}

	for i := 0; i < number_of_clients; i++ {
		go func(i int) {
			client_config := configs[i]
			filename := fmt.Sprintf("pool_client.yaml.%d", i)
			client_config.Client.WritebackLinux = path.Join(
				*pool_client_writeback_dir, filename)

			client_config.Client.WritebackWindows = client_config.Client.WritebackLinux
			if client_config.Client.LocalBuffer != nil {
				client_config.Client.LocalBuffer.DiskSize = 0
			}
			client_config.Client.Concurrency = uint64(*pool_client_concurrency)

			existing_writeback := &config_proto.Writeback{}
			writeback, err := config.WritebackLocation(client_config)
			kingpin.FatalIfError(err, "Unable to load writeback file")

			data, err := ioutil.ReadFile(writeback)

			// Failing to read the file is not an error - the file may not
			// exist yet.
			if err == nil || len(data) == 0 {
				err = yaml.Unmarshal(data, existing_writeback)
				if err != nil {
					fmt.Printf("Unable to load config file %v: %v", filename, err)
					existing_writeback = &config_proto.Writeback{}
				}
			}

			// Merge the writeback with the config.
			client_config.Writeback = existing_writeback

			// Force new events to be read from the server
			client_config.Writeback.EventQueries = nil

			// Make sure the config is ok.
			err = crypto_utils.VerifyConfig(client_config)
			if err != nil {
				kingpin.FatalIfError(err, "Invalid config")
			}

			manager, err := crypto_client.NewClientCryptoManager(
				client_config, []byte(client_config.Writeback.PrivateKey))
			kingpin.FatalIfError(err, "Unable to parse config file")

			exe, err := executor.NewPoolClientExecutor(ctx, client_config, i)
			kingpin.FatalIfError(err, "Can not create executor.")

			comm, err := http_comms.NewHTTPCommunicator(
				client_config,
				manager,
				exe,
				client_config.Client.ServerUrls,
				nil,
				utils.RealClock{},
			)
			kingpin.FatalIfError(err, "Can not create HTTPCommunicator.")

			c.Inc()
			// Run the client in the background.
			comm.Run(ctx)
		}(i)
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
