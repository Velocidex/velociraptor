/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

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

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/http_comms"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services/writeback"
	"www.velocidex.com/golang/velociraptor/startup"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	"www.velocidex.com/golang/velociraptor/vql/tools"
)

var (
	// Run the client.
	client            = app.Command("client", "Run the velociraptor client")
	client_quiet_flag = client.Flag("quiet",
		"Do not output anything to stdout/stderr").Bool()
	client_admin_flag = client.Flag("require_admin", "Ensure the user is an admin").Bool()
)

func doClient() error {
	if *client_admin_flag {
		err := checkAdmin()
		if err != nil {
			return err
		}
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	// Include the writeback in the client's configuration.
	config_obj, err := makeDefaultConfigLoader().
		WithRequiredClient().
		WithRequiredLogging().
		WithFileLoader(*config_path).
		WithWriteback().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	err = InstallAuditlogger()
	if err != nil {
		return err
	}

	return RunClient(ctx, config_obj)
}

// Run the client - if the client exits restart it.
func RunClient(
	ctx context.Context,
	config_obj *config_proto.Config) error {

	err := checkMutex()
	if err != nil {
		return err
	}

	for {
		subctx, cancel := context.WithCancel(ctx)
		lwg := &sync.WaitGroup{}

		lwg.Add(1)
		go func() {
			err := runClientOnce(subctx, lwg, config_obj)
			if err != nil {
				logger := logging.GetLogger(config_obj, &logging.ClientComponent)
				logger.Error("<red>runClientOnce Error:</> %v", err)
			}
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
	config_obj *config_proto.Config) (err error) {

	defer wg.Done()

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

	tempfile.SetTempfile(config_obj)

	writeback_service := writeback.GetWritebackService()
	writeback, err := writeback_service.GetWriteback(config_obj)
	if err != nil {
		return err
	}

	sm, err := startup.StartClientServices(ctx, config_obj, on_error)
	defer sm.Close()
	if err != nil {
		return err
	}

	exe, err := executor.NewClientExecutor(ctx, writeback.ClientId, config_obj)
	if err != nil {
		return fmt.Errorf("Can not create executor: %w", err)
	}

	_, err = http_comms.StartHttpCommunicatorService(
		ctx, sm.Wg, config_obj, exe, on_error)
	if err != nil {
		return err
	}

	// Check for crashes
	err = executor.RunStartupTasks(ctx, config_obj, sm.Wg, exe)
	if err != nil {
		return err
	}

	<-ctx.Done()
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

			FatalIfError(client, doClient)
			return true
		}
		return false
	})
}
