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
	"os"
	"sync"

	"www.velocidex.com/golang/velociraptor/config"
	crypto_client "www.velocidex.com/golang/velociraptor/crypto/client"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/http_comms"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/orgs"
	"www.velocidex.com/golang/velociraptor/services/repository"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/tools"
)

var (
	// Run the client.
	client            = app.Command("client", "Run the velociraptor client")
	client_quiet_flag = client.Flag("quiet",
		"Do not output anything to stdout/stderr").Bool()
)

func RunClient(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_path *string) error {

	defer wg.Done()

	err := checkMutex()
	if err != nil {
		return err
	}

	for {
		subctx, cancel := context.WithCancel(ctx)
		lwg := &sync.WaitGroup{}

		lwg.Add(1)
		go func() {
			runClientOnce(subctx, lwg, config_path)
			cancel()
		}()

		select {
		case <-subctx.Done():
			// Wait for the client to shutdown before we exit.
			cancel()
			lwg.Wait()
			return nil

		case <-tools.ClientRestart:
			cancel()
			// Wait for the client to shutdown before we restart it.
			lwg.Wait()
		}
	}
}

func runClientOnce(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_path *string) (err error) {
	defer wg.Done()

	lwg := &sync.WaitGroup{}

	// Include the writeback in the client's configuration.
	config_obj, err := makeDefaultConfigLoader().
		WithRequiredClient().
		WithRequiredLogging().
		WithFileLoader(*config_path).
		WithWriteback().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	// Report any errors from this function.
	defer func() {
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.ClientComponent)
			logger.Error("<red>runClientOnce Error:</> %v", err)
		}
	}()

	// Make sure the config crypto is ok.
	err = crypto_utils.VerifyConfig(config_obj)
	if err != nil {
		return fmt.Errorf("Invalid config: %w", err)
	}

	executor.SetTempfile(config_obj)

	writeback, err := config.GetWriteback(config_obj.Client)
	if err != nil {
		return err
	}

	manager, err := crypto_client.NewClientCryptoManager(
		config_obj, []byte(writeback.PrivateKey))
	if err != nil {
		return err
	}

	// Start all the services
	sm := services.NewServiceManager(ctx, config_obj)
	defer sm.Close()

	// Start the nanny first so we are covered from here on.
	err = sm.Start(executor.StartNannyService)
	if err != nil {
		return err
	}

	err = sm.Start(orgs.StartClientOrgManager)
	if err != nil {
		return err
	}

	// Start the repository manager before we can handle any VQL
	repo_manager, _ := services.GetRepositoryManager()
	if repo_manager == nil {
		err = sm.Start(repository.StartRepositoryManager)
		if err != nil {
			return err
		}
	}

	exe, err := executor.NewClientExecutor(ctx, config_obj)
	if err != nil {
		return fmt.Errorf("Can not create executor: %w", err)
	}

	// Now start the communicator so we can talk with the server.
	comm, err := http_comms.NewHTTPCommunicator(
		ctx,
		config_obj,
		manager,
		exe,
		config_obj.Client.ServerUrls,
		func() { on_error(ctx, config_obj) },
		utils.RealClock{},
	)
	if err != nil {
		return fmt.Errorf("Can not create HTTPCommunicator: %w", err)
	}

	lwg.Add(1)
	go comm.Run(ctx, lwg)

	// Start services **after** the communicator is up in case
	// services need to send messages.
	err = executor.StartServices(sm, manager.ClientId, exe)
	if err != nil {
		return fmt.Errorf("Starting services: %w", err)
	}

	lwg.Add(1)
	go func() {
		defer lwg.Done()
		<-ctx.Done()

		logger := logging.GetLogger(config_obj, &logging.ClientComponent)
		logger.Info("<cyan>Interrupted!</> Shutting down\n")
	}()

	lwg.Wait()

	return nil
}

func maybeCloseOutput() {
	if *client_quiet_flag {
		os.Stdout.Close()
		os.Stderr.Close()
	}
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		if command == client.FullCommand() {
			maybeCloseOutput()

			wg := &sync.WaitGroup{}
			ctx, cancel := install_sig_handler()
			defer cancel()

			FatalIfError(client, func() error {
				wg.Add(1)
				return RunClient(ctx, wg, config_path)
			})

			return true
		}
		return false
	})
}
