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
	"github.com/sirupsen/logrus"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
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
)

func doFrontend() {
	config_obj, err := makeDefaultConfigLoader().
		WithRequiredFrontend().
		WithRequiredUser().
		WithRequiredLogging().LoadAndValidate()
	kingpin.FatalIfError(err, "Unable to load config file")

	ctx, cancel := install_sig_handler()
	defer cancel()

	if *frontend_cmd_node != "" {
		kingpin.FatalIfError(frontend.SelectFrontend(
			*frontend_cmd_node, config_obj),
			"Selecting minion frontend")
	}

	sm := services.NewServiceManager(ctx, config_obj)
	defer sm.Close()

	server, err := startFrontend(sm)
	kingpin.FatalIfError(err, "startFrontend %+vv", err)
	defer server.Close()

	sm.Wg.Wait()
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

	// Load the assets into memory.
	assets.Init()

	// Increase resource limits.
	server.IncreaseLimits(config_obj)

	// Start the frontend service if needed. This must happen
	// first so other services can contact the master node.

	config_obj.Frontend.IsMaster = !*frontend_cmd_minion
	err := sm.Start(frontend.StartFrontendService)
	if err != nil {
		return nil, err
	}

	// These services must start on all frontends
	err = startup.StartupEssentialServices(sm)
	if err != nil {
		return nil, err
	}

	// These services must start only on the frontends.
	err = startup.StartupFrontendServices(sm)
	if err != nil {
		return nil, err
	}

	// Parse the artifacts database to detect errors early.
	_, err = getRepository(config_obj)
	if err != nil {
		return nil, err
	}

	server_builder, err := api.NewServerBuilder(sm.Ctx, config_obj, sm.Wg)
	if err != nil {
		return nil, err
	}

	// Start the gRPC API server.
	if config_obj.Frontend.ServerServices.ApiServer {
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
			doFrontend()
			return true
		}
		return false
	})
}
