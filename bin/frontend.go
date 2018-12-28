// Frontend is not built on Windows.
package main

import (
	"net/http"

	"github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/velociraptor/api"
	"www.velocidex.com/golang/velociraptor/gui/assets"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/server"
)

var (
	// Run the server.
	frontend = app.Command("frontend", "Run the frontend and GUI.")
)

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		if command == frontend.FullCommand() {
			config_obj, err := get_server_config(*config_path)
			kingpin.FatalIfError(err, "Unable to load config file")

			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.WithFields(logrus.Fields{
				"version":    config_obj.Version.Version,
				"build_time": config_obj.Version.BuildTime,
				"commit":     config_obj.Version.Commit,
			}).Info("Starting Frontend.")

			server_obj, err := server.NewServer(config_obj)
			kingpin.FatalIfError(err, "Unable to create server")
			defer server_obj.Close()

			// Parse the artifacts database to detect errors early.
			getRepository(config_obj)

			// Load the assets into memory.
			assets.Init()

			// Increase resource limits.
			server.IncreaseLimits(config_obj)

			// Start the gRPC API server.
			go func() {
				err := api.StartServer(config_obj, server_obj)
				kingpin.FatalIfError(
					err, "Unable to start API server")
			}()

			if config_obj.AutocertDomain == "" {
				// For non TLS we separate the GUI and
				// frontend ports because the frontend
				// must be publically accessible but
				// the GUI must only be accessed over
				// 127.0.0.1 without TLS.
				go func() {
					router := http.NewServeMux()
					err := api.PrepareMux(config_obj, router)
					kingpin.FatalIfError(
						err, "Unable to start API server")

					// Start the GUI separately on
					// a different port.
					err = api.StartHTTPProxy(config_obj, router)
					kingpin.FatalIfError(
						err, "Unable to start GUI server")
				}()

				// Add Frontend Comms handlers.
				router := http.NewServeMux()
				server.PrepareFrontendMux(config_obj, server_obj, router)

				// Start comms over http.
				err = server.StartFrontendHttp(config_obj, server_obj, router)
				kingpin.FatalIfError(err, "StartFrontendHttp")

			} else {
				// For autocert we combine the GUI and
				// frontends on the same port. The
				// ACME protocol requires ports 80 and
				// 443 for all services.
				router := http.NewServeMux()
				err := api.PrepareMux(config_obj, router)
				kingpin.FatalIfError(
					err, "Unable to start API server")

				// Add Comms handlers.
				server.PrepareFrontendMux(config_obj, server_obj, router)

				// Block here until done.
				err = server.StartTLSServer(config_obj, server_obj, router)
				kingpin.FatalIfError(err, "StartTLSServer")
			}
			return true
		}
		return false
	})
}
