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
	"sync"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/crypto"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/http_comms"
	logging "www.velocidex.com/golang/velociraptor/logging"
)

var (
	// Run the client.
	client = app.Command("client", "Run the velociraptor client")
)

func RunClient(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_path *string) {
	config_obj, err := config.LoadClientConfig(*config_path)
	kingpin.FatalIfError(err, "Unable to load config file")

	// Make sure the config is ok.
	err = crypto.VerifyConfig(config_obj)
	if err != nil {
		kingpin.FatalIfError(err, "Invalid config")
	}

	manager, err := crypto.NewClientCryptoManager(
		config_obj, []byte(config_obj.Writeback.PrivateKey))
	if err != nil {
		kingpin.FatalIfError(err, "Unable to parse config file")
	}

	exe, err := executor.NewClientExecutor(ctx, config_obj)
	if err != nil {
		kingpin.FatalIfError(err, "Can not create executor.")
	}

	comm, err := http_comms.NewHTTPCommunicator(
		config_obj,
		manager,
		exe,
		config_obj.Client.ServerUrls,
	)
	kingpin.FatalIfError(err, "Can not create HTTPCommunicator.")

	// Wait for all services to properly start before we begin the
	// comms.
	executor.StartServices(config_obj, manager.ClientId, exe)

	wg.Add(1)
	go func() {
		defer wg.Done()

		comm.Run(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()

		logger := logging.GetLogger(config_obj, &logging.ClientComponent)
		logger.Info("Interrupted! Shutting down\n")
	}()
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		if command == client.FullCommand() {
			wg := &sync.WaitGroup{}
			ctx, cancel := install_sig_handler()
			defer cancel()

			RunClient(ctx, wg, config_path)

			wg.Wait()

			return true
		}
		return false
	})
}
