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
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/gui/assets"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/server"
	"www.velocidex.com/golang/velociraptor/services"
)

var (
	// Run the server.
	frontend_cmd     = app.Command("frontend", "Run the frontend and GUI.")
	compression_flag = frontend_cmd.Flag("disable_artifact_compression",
		"Disables artifact compressions").Bool()
	frontend_node = frontend_cmd.Flag("node", "Run this specified node only").
			String()
)

func doFrontend() {
	config_obj, err := DefaultConfigLoader.
		WithRequiredFrontend().
		WithRequiredUser().
		WithRequiredLogging().LoadAndValidate()
	kingpin.FatalIfError(err, "Unable to load config file")

	ctx, cancel := install_sig_handler()
	defer cancel()

	sm := services.NewServiceManager(ctx, config_obj)
	defer sm.Close()

	server, err := startFrontend(sm, config_obj)
	kingpin.FatalIfError(err, "startFrontend")
	defer server.Close()

	sm.Wg.Wait()
}

// Start the frontend service.
func startFrontend(sm *services.Service,
	config_obj *config_proto.Config) (*api.Builder, error) {

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

	// Parse the artifacts database to detect errors early.
	_, err := getRepository(config_obj)
	kingpin.FatalIfError(err, "Loading extra artifacts")

	// Load the assets into memory.
	assets.Init()

	// Increase resource limits.
	server.IncreaseLimits(config_obj)

	// These services must start on all frontends
	err = server.StartFrontendServices(config_obj, sm, *frontend_node)
	if err != nil {
		logger.Error("Failed starting services: ", err)
		return nil, err
	}

	server_builder, err := api.NewServerBuilder(config_obj)
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
