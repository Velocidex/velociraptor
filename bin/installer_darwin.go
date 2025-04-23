//go:build darwin
// +build darwin

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
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	errors "github.com/go-errors/errors"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	service_command = app.Command(
		"service", "Manipulate the Velociraptor service.")
	install_command = service_command.Command(
		"install", "Install Velociraptor as a service.")
	remove_command = service_command.Command(
		"remove", "Remove the Velociraptor service.")
)

func doRemove() error {
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().WithRequiredClient().
		WithWriteback().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	if config_obj.Client.DarwinInstaller == nil {
		return errors.New("DarwinInstaller not configured")
	}

	service_name := config_obj.Client.DarwinInstaller.ServiceName
	plist_path := "/Library/LaunchDaemons/" + service_name + ".plist"
	err = exec.CommandContext(context.Background(),
		"/bin/launchctl", "unload", "-w", plist_path).Run()
	if err != nil {
		return fmt.Errorf("Can't load service: %w", err)
	}
	return nil
}

func doInstall() error {
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().WithRequiredClient().
		WithWriteback().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("Can't get executable path: %w", err)
	}

	service_name := config_obj.Client.DarwinInstaller.ServiceName
	logger := logging.GetLogger(config_obj, &logging.ClientComponent)
	target_path := utils.ExpandEnv(config_obj.Client.DarwinInstaller.InstallPath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Try to copy the executable to the target_path.
	err = utils.CopyFile(ctx, executable, target_path, 0755)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		dirname := filepath.Dir(target_path)
		logger.Info("Attempting to create intermediate directory %s.",
			dirname)
		err = os.MkdirAll(dirname, 0755)
		if err != nil {
			return fmt.Errorf("Create intermediate directories: %w", err)
		}
		err = utils.CopyFile(ctx, executable, target_path, 0755)
	}
	if err != nil {
		return fmt.Errorf("Cant copy binary into destination dir: %w", err)
	}

	logger.Info("Copied binary to %s", target_path)

	config_path_plist := ""

	// If the installer was invoked with the --config arg then we need
	// to copy the config to the target path. Otherwise the config may
	// be embedded so we dont need to use it at all.
	if *config_path != "" {
		config_target_path := strings.TrimSuffix(
			target_path, filepath.Ext(target_path)) + ".config.yaml"

		logger.Info("Copying config to destination %s",
			config_target_path)

		err = utils.CopyFile(ctx, *config_path, config_target_path, 0755)
		if err != nil {
			logger.Info("Cant copy config to destination %s: %v",
				config_target_path, err)
			return err
		}
		config_path_plist = fmt.Sprintf(`
                <string>--config</string>
                <string>%v.config.yaml</string>
`, target_path)
	}

	plist_path := "/Library/LaunchDaemons/" + service_name + ".plist"
	plist := fmt.Sprintf(`
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple Computer//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
        <key>Label</key>
        <string>%v</string>
        <key>ProgramArguments</key>
        <array>
                <string>%v</string>
                <string>client</string>
%v
                <string>--quiet</string>
        </array>
        <key>KeepAlive</key>
        <true/>
</dict>
</plist>`, service_name, target_path, config_path_plist)

	err = ioutil.WriteFile(plist_path, []byte(plist), 0644)
	if err != nil {
		return fmt.Errorf("Can't write plist file: %w", err)
	}

	err = exec.CommandContext(context.Background(),
		"/bin/launchctl", "load", "-w", plist_path).Run()
	if err != nil {
		return fmt.Errorf("Can't load service: %w", err)
	}

	// We need to kill the service so it can restart with the new
	// settings. Use SIGINT to allow it to cleanup.
	err = exec.CommandContext(context.Background(),
		"/bin/launchctl", "kill", "SIGINT", "system/"+service_name).Run()
	if err != nil {
		return fmt.Errorf("Can't restart service: %w", err)
	}
	return nil
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case install_command.FullCommand():
			FatalIfError(install_command, doInstall)

		case remove_command.FullCommand():
			FatalIfError(remove_command, doRemove)

		default:
			return false
		}
		return true
	})
}
