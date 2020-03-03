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
	"net/http"
	"os/user"
	"sync"

	errors "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/velociraptor/api"
	"www.velocidex.com/golang/velociraptor/gui/assets"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/notifications"
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

	server, err := startFrontend(ctx, wg, config_obj)
	kingpin.FatalIfError(err, "startFrontend")
	defer server.Close()

	// Wait here until everything is done.
	wg.Wait()
}

// Start the frontend service.
func startFrontend(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (*server.Server, error) {

	err := checkFrontendUser(config_obj)
	if err != nil {
		return nil, err
	}

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

	var notifier *notifications.NotificationPool
	var server_obj *server.Server

	if config_obj.ServerServices.FrontendServer ||
		config_obj.ServerServices.ApiServer {
		// Create a new server
		server_obj, err = server.NewServer(config_obj)
		kingpin.FatalIfError(err, "Unable to create server")

		notifier = server_obj.NotificationPool
	}

	// Start Server Services
	err = services.StartServices(
		ctx, wg, config_obj, notifier)
	if err != nil {
		logger.Error("Failed starting services: ", err)
		return nil, err
	}

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

	// The below configures the frontend or gui services.
	if !config_obj.ServerServices.FrontendServer &&
		!config_obj.ServerServices.GuiServer {
		return server_obj, nil
	}

	// Are we in autocert mode? There are special requirements in
	// this case.
	if config_obj.AutocertDomain != "" {
		startAutoCertFrontend(ctx, wg, config_obj, server_obj)

		// If the GUI and Frontend need to be on the same port
		// we just merge the handlers and start one server.
	} else if config_obj.GUI.BindAddress == config_obj.Frontend.BindAddress &&
		config_obj.GUI.BindPort == config_obj.Frontend.BindPort {
		startSharedSelfSignedFrontend(ctx, wg, config_obj, server_obj)

		// Launch the frontend and gui on different ports.
	} else {
		startSelfSignedFrontend(ctx, wg, config_obj, server_obj)
	}

	return server_obj, nil
}

// When the GUI and Frontend share the same port we start them with
// the same server.
func startSharedSelfSignedFrontend(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config,
	server_obj *server.Server) {
	mux := http.NewServeMux()

	if config_obj.ServerServices.FrontendServer {
		server.PrepareFrontendMux(
			config_obj, server_obj, mux)
	}

	router, err := api.PrepareMux(config_obj, mux)
	kingpin.FatalIfError(
		err, "Unable to start API server")

	// Start comms over https.
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

// Start the Frontend and GUI on different ports using different
// server objects.
func startSelfSignedFrontend(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config,
	server_obj *server.Server) {

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

// When in autocert mode, we share the same port for both frontend and
// gui. We also ignore the port settings because letsencrypt must use
// port 443 and 80.
func startAutoCertFrontend(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config,
	server_obj *server.Server) {
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
}

func checkFrontendUser(config_obj *config_proto.Config) error {
	if config_obj.Frontend.RunAsUser == "" {
		return nil
	}

	user, err := user.Current()
	if err != nil {
		return err
	}

	if user.Username != config_obj.Frontend.RunAsUser {
		return errors.New(fmt.Sprintf(
			"Velociraptor should be running as the '%s' user but you are '%s'. "+
				"Please change user with sudo first.",
			config_obj.Frontend.RunAsUser, user.Username))
	}

	return nil
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
