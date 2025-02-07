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
	"fmt"

	"github.com/sirupsen/logrus"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/server"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/startup"
)

var (
	// Run the server.
	frontend_cmd     = app.Command("frontend", "Run the frontend and GUI.")
	compression_flag = frontend_cmd.Flag("disable_artifact_compression",
		"Disables artifact compressions").Bool()
	frontend_cmd_minion = frontend_cmd.Flag("minion", "This is a minion frontend").Bool()

	frontend_cmd_node = frontend_cmd.Flag("node", "The name of a minion - selects from available frontend configurations (DEPRECATED: ignored)").String()

	frontend_disable_panic_guard = frontend_cmd.Flag("disable-panic-guard",
		"Disabled the panic guard mechanism (not recommended)").Bool()
)

func doFrontendWithPanicGuard() error {
	if !*frontend_disable_panic_guard {
		err := writeLogOnPanic()
		if err != nil {
			return err
		}
		// Do the banner again as it will get flushed with the pre
		// logs.
		doBanner()
	}
	return doFrontend()
}

// Start the frontend
func doFrontend() error {
	config_obj, err := makeDefaultConfigLoader().
		WithRequiredFrontend().
		WithRequiredUser().
		WithRequiredLogging().LoadAndValidate()
	if err != nil {
		logging.FlushPrelogs(config.GetDefaultConfig())
		return fmt.Errorf("loading config file: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	// Come up with a suitable services plan depending on the frontend
	// role.
	if config_obj.Services == nil {
		if *frontend_cmd_minion {
			config_obj.Services = services.MinionServicesSpec()
		} else {
			config_obj.Services = services.AllServerServicesSpec()
		}
	}

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

	// Increase resource limits.
	server.IncreaseLimits(config_obj)

	// Minions use the RemoteFileDataStore to sync with the server.
	if !services.IsMaster(config_obj) {
		logger.Info("Frontend will run as a <green>minion</>.")
		logger.Info("<green>Enabling remote datastore</> since we are a minion.")
		config_obj.Datastore.Implementation = "RemoteFileDataStore"
	}

	// Now start the frontend services
	sm, err := startup.StartFrontendServices(ctx, config_obj)
	if err != nil {
		return fmt.Errorf("starting frontend: %w", err)
	}
	defer sm.Close()

	// Wait here for completion.
	sm.Wg.Wait()

	return nil
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		if command == frontend_cmd.FullCommand() {
			FatalIfError(frontend_cmd, doFrontendWithPanicGuard)
		}
		return false
	})
}
