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
	"net/http"
	"sync"

	"github.com/sirupsen/logrus"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/velociraptor/api"
	"www.velocidex.com/golang/velociraptor/gui/assets"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/server"
	"www.velocidex.com/golang/velociraptor/services"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

var (
	// Run the server.
	frontend         = app.Command("frontend", "Run the frontend and GUI.")
	compression_flag = frontend.Flag("disable_artifact_compression",
		"Disables artifact compressions").Bool()
)

func doFrontend() {
	config_obj, err := get_server_config(*config_path)
	kingpin.FatalIfError(err, "Unable to load config file")

	// Use both context and WaitGroup to control life time of
	// services.
	wg := &sync.WaitGroup{}
	ctx, cancel := install_sig_handler()
	defer cancel()

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.WithFields(logrus.Fields{
		"version":    config_obj.Version.Version,
		"build_time": config_obj.Version.BuildTime,
		"commit":     config_obj.Version.Commit,
	}).Info("Starting Frontend.")

	if *compression_flag {
		logger.Info("Disabling artifact compression.")
		config_obj.Frontend.DoNotCompressArtifacts = true
	}

	// If not specified at all, start all services on this instance.
	if config_obj.ServerServices == nil {
		config_obj.ServerServices = &config_proto.ServerServicesConfig{
			HuntManager:       true,
			HuntDispatcher:    true,
			StatsCollector:    true,
			ServerMonitoring:  true,
			ServerArtifacts:   true,
			DynDns:            true,
			Interrogation:     true,
			SanityChecker:     true,
			VfsService:        true,
			UserManager:       true,
			ClientMonitoring:  true,
			MonitoringService: true,
			ApiServer:         true,
			FrontendServer:    true,
			GuiServer:         true,
		}
	}

	// Parse the artifacts database to detect errors early.
	getRepository(config_obj)

	// Load the assets into memory.
	assets.Init()

	// Increase resource limits.
	server.IncreaseLimits(config_obj)

	// Create a new server
	server_obj, err := server.NewServer(config_obj)
	kingpin.FatalIfError(err, "Unable to create server")
	defer server_obj.Close()

	// Start Server Services
	manager, err := services.StartServices(
		ctx, wg, config_obj,
		server_obj.NotificationPool)
	if err != nil {
		logger.Error("Failed starting services: ", err)
		return
	}
	defer manager.Close()

	// Start monitoring.
	if config_obj.ServerServices.MonitoringService {
		api.StartMonitoringService(ctx, wg, config_obj)
	}

	// Start the gRPC API server.
	if config_obj.ServerServices.ApiServer {
		wg.Add(1)
		go func() {
			defer wg.Done()

			err := api.StartServer(ctx, wg, config_obj, server_obj)
			kingpin.FatalIfError(
				err, "Unable to start API server")
		}()
	}

	// Are we in autocert mode?
	if config_obj.AutocertDomain != "" {
		// For autocert we combine the GUI and
		// frontends on the same port. The
		// ACME protocol requires ports 80 and
		// 443 for all services.
		mux := http.NewServeMux()

		// Add Comms handlers.
		if config_obj.ServerServices.FrontendServer {
			server.PrepareFrontendMux(
				config_obj, server_obj, mux)
		}

		router, err := api.PrepareMux(config_obj, mux)
		kingpin.FatalIfError(
			err, "Unable to start API server")

		wg.Add(1)
		go func() {
			defer wg.Done()

			err = server.StartTLSServer(
				ctx, wg, config_obj, server_obj, router)
			kingpin.FatalIfError(err, "StartTLSServer")
		}()
	} else {
		// Otherwise by default we use self signed SSL.
		// If the GUI and Frontend need to be on the same
		// server we just merge the handlers.
		if config_obj.GUI.BindAddress == config_obj.Frontend.BindAddress &&
			config_obj.GUI.BindPort == config_obj.Frontend.BindPort {

			mux := http.NewServeMux()

			if config_obj.ServerServices.FrontendServer {
				server.PrepareFrontendMux(
					config_obj, server_obj, mux)
			}

			router, err := api.PrepareMux(config_obj, mux)
			kingpin.FatalIfError(
				err, "Unable to start API server")

			// Start comms over https.
			if config_obj.ServerServices.FrontendServer ||
				config_obj.ServerServices.GuiServer {

				wg.Add(1)
				go func() {
					defer wg.Done()
					err = server.StartFrontendHttps(
						ctx, wg,
						config_obj, server_obj, router)
					kingpin.FatalIfError(
						err, "StartFrontendHttps")
				}()
			}

		} else {
			// Launch a new server for the GUI.
			if config_obj.ServerServices.GuiServer {
				wg.Add(1)

				go func() {
					defer wg.Done()
					mux := http.NewServeMux()

					router, err := api.PrepareMux(config_obj, mux)
					kingpin.FatalIfError(
						err, "Unable to start API server")

					// Start the GUI separately on
					// a different port.
					api.StartSelfSignedHTTPSProxy(
						ctx, wg, config_obj, router)
				}()
			}

			// Add Comms handlers if required.
			if config_obj.ServerServices.FrontendServer {
				wg.Add(1)

				go func() {
					defer wg.Done()
					// Launch a server for the frontend.
					mux := http.NewServeMux()

					server.PrepareFrontendMux(
						config_obj, server_obj, mux)

					// Start comms over https.
					err := server.StartFrontendHttps(
						ctx, wg,
						config_obj, server_obj, mux)
					kingpin.FatalIfError(err, "StartFrontendHttps")
				}()
			}
		}
	}

	// Wait here until everything is done.
	wg.Wait()
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		if command == frontend.FullCommand() {
			doFrontend()
			return true
		}
		return false
	})
}
