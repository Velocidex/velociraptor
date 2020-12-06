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
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	// Run the client.
	client = app.Command("client", "Run the velociraptor client")
)

func RunClient(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_path *string) {

	// Include the writeback in the client's configuration.
	config_obj, err := new(config.Loader).
		WithVerbose(*verbose_flag).
		WithFileLoader(*config_path).
		WithEmbedded().
		WithEnvLoader("VELOCIRAPTOR_CONFIG").
		WithCustomValidator(initFilestoreAccessor).
		WithCustomValidator(initDebugServer).
		WithLogFile(*logging_flag).
		WithRequiredClient().
		WithRequiredLogging().
		WithWriteback().LoadAndValidate()
	kingpin.FatalIfError(err, "Unable to load config file")

	// Make sure the config crypto is ok.
	err = crypto.VerifyConfig(config_obj)
	if err != nil {
		kingpin.FatalIfError(err, "Invalid config")
	}

	executor.SetTempfile(config_obj)

	manager, err := crypto.NewClientCryptoManager(
		config_obj, []byte(config_obj.Writeback.PrivateKey))
	if err != nil {
		kingpin.FatalIfError(err, "Unable to parse config file")
	}

	// Start all the services
	sm := services.NewServiceManager(ctx, config_obj)
	defer sm.Close()

	exe, err := executor.NewClientExecutor(ctx, config_obj)
	if err != nil {
		kingpin.FatalIfError(err, "Can not create executor.")
	}

	err = executor.StartServices(sm, manager.ClientId, exe)
	if err != nil {
		kingpin.FatalIfError(err, "Can not start services.")
	}

	// Now start the communicator so we can talk with the server.
	comm, err := http_comms.NewHTTPCommunicator(
		config_obj,
		manager,
		exe,
		config_obj.Client.ServerUrls,
		func() { on_error(config_obj) },
		utils.RealClock{},
	)
	kingpin.FatalIfError(err, "Can not create HTTPCommunicator.")

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
		logger.Info("<cyan>Interrupted!</> Shutting down\n")
	}()

	wg.Wait()
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		if command == client.FullCommand() {
			wg := &sync.WaitGroup{}
			ctx, cancel := install_sig_handler()
			defer cancel()

			RunClient(ctx, wg, config_path)

			return true
		}
		return false
	})
}
