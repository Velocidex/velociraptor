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
	"fmt"

	"github.com/sirupsen/logrus"
	"www.velocidex.com/golang/velociraptor/api"
	assets "www.velocidex.com/golang/velociraptor/gui/velociraptor"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/server"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/frontend"
	"www.velocidex.com/golang/velociraptor/startup"
)

var (
	// Run the server.
	frontend_cmd     = app.Command("frontend", "Run the frontend and GUI.")
	compression_flag = frontend_cmd.Flag("disable_artifact_compression",
		"Disables artifact compressions").Bool()
	frontend_cmd_minion = frontend_cmd.Flag("minion", "This is a minion frontend").Bool()

	frontend_cmd_node = frontend_cmd.Flag("node", "The name of a minion - selects from available frontend configurations").String()

	frontend_disable_panic_guard = frontend_cmd.Flag("disable-panic-guard",
		"Disabled the panic guard mechanism (not recommended)").Bool()
)

func doFrontendWithPanicGuard() error {
	if !*frontend_disable_panic_guard {
		err := writeLogOnPanic()
		if err != nil {
			return err
		}
	}
	return doFrontend()
}

func doFrontend() error {
	config_obj, err := makeDefaultConfigLoader().
		WithRequiredFrontend().
		WithRequiredUser().
		WithRequiredLogging().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("loading config file: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	sm := services.NewServiceManager(ctx, config_obj)
	defer sm.Close()

	server, err := startFrontend(sm)
	if err != nil {
		return fmt.Errorf("starting frontend: %w", err)
	}
	defer server.Close()

	// Wait here for completion.
	sm.Wg.Wait()

	return nil
}

// Start the frontend service.
func startFrontend(sm *services.Service) (*api.Builder, error) {
	config_obj := sm.Config

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.WithFields(logrus.Fields{
		"version":    config_obj.Version.Version,
		"build_time": config_obj.Version.BuildTime,
		"commit":     config_obj.Version.Commit,
	}).Info("<green>Starting</> Frontend.")

	if *compression_flag {
		logger.Info("Disabling artifact compression.")
		config_obj.Frontend.DoNotCompressArtifacts = true
	}

	// Load the assets into memory if we are the master node.
	if services.IsMaster(config_obj) {
		assets.Init()
	}

	// Increase resource limits.
	server.IncreaseLimits(config_obj)

	// Minions use the RemoteFileDataStore to sync with the server.
	if !services.IsMaster(config_obj) {
		logger.Info("Frontend will run as a <green>minion</>.")
		logger.Info("<green>Enabling remote datastore</> since we are a minion.")
		config_obj.Datastore.Implementation = "RemoteFileDataStore"
	}

	err := sm.Start(frontend.StartFrontendService)
	if err != nil {
		return nil, err
	}

	// These services must start on all frontends
	err = startup.StartupEssentialServices(sm)
	if err != nil {
		return nil, err
	}

	// Parse extra artifacts from --definitions flag before we start
	// any services just in case these services need to access these
	// custom artifacts.
	_, err = getRepository(config_obj)
	if err != nil {
		return nil, err
	}

	// Load any artifacts defined in the config file before the
	// frontend services are started so they may use them.
	err = load_config_artifacts(config_obj)
	if err != nil {
		return nil, err
	}

	// These services must start only on the frontends.
	err = startup.StartupFrontendServices(sm)
	if err != nil {
		return nil, err
	}

	server_builder, err := api.NewServerBuilder(sm.Ctx, config_obj, sm.Wg)
	if err != nil {
		return nil, err
	}

	// Start the gRPC API server on the master only.
	if services.IsMaster(config_obj) {
		err = server_builder.WithAPIServer(sm.Ctx, sm.Wg)
		if err != nil {
			return nil, err
		}
	}

	return server_builder, server_builder.StartServer(sm.Ctx, sm.Wg)
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		if command == frontend_cmd.FullCommand() {
			FatalIfError(frontend_cmd, doFrontendWithPanicGuard)
		}
		return false
	})
}
