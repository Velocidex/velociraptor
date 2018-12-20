// +build darwin

package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	errors "github.com/pkg/errors"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	config "www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	service_command = app.Command(
		"service", "Manipulate the Velociraptor service.")
	installl_command = service_command.Command(
		"install", "Install Velociraptor as a Windows service.")
)

func doInstall() error {
	config_obj, err := config.LoadClientConfig(*config_path)
	if err != nil {
		return errors.Wrap(err, "Unable to load config file")
	}

	executable, err := os.Executable()
	kingpin.FatalIfError(err, "Can't get executable path")

	service_name := config_obj.Client.DarwinInstaller.ServiceName
	logger := logging.GetLogger(config_obj, &logging.ClientComponent)
	target_path := os.ExpandEnv(config_obj.Client.DarwinInstaller.InstallPath)

	// Try to copy the executable to the target_path.
	err = utils.CopyFile(executable, target_path, 0755)
	if err != nil && os.IsNotExist(errors.Cause(err)) {
		dirname := filepath.Dir(target_path)
		logger.Info("Attempting to create intermediate directory %s.",
			dirname)
		err = os.MkdirAll(dirname, 0700)
		if err != nil {
			return errors.Wrap(err, "Create intermediate directories")
		}
		err = utils.CopyFile(executable, target_path, 0755)
	}
	if err != nil {
		return errors.Wrap(err, "Cant copy binary into destination dir.")
	}

	logger.Info("Copied binary to %s", target_path)

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
        </array>
        <key>KeepAlive</key>
        <true/>
</dict>
</plist>`, service_name, target_path)

	err = ioutil.WriteFile(plist_path, []byte(plist), 0644)
	kingpin.FatalIfError(err, "Can't write plist file.")

	err = exec.CommandContext(context.Background(),
		"/bin/launchctl", "load", "-w", plist_path).Run()
	kingpin.FatalIfError(err, "Can't load service.")

	// We need to kill the service so it can restart with the new
	// settings. Use SIGINT to allow it to cleanup.
	err = exec.CommandContext(context.Background(),
		"/bin/launchctl", "kill", "SIGINT", "system/"+service_name).Run()
	kingpin.FatalIfError(err, "Can't restart service.")

	return nil
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		var err error
		switch command {
		case "service install":
			err = doInstall()
		default:
			return false
		}

		kingpin.FatalIfError(err, "")
		return true
	})
}
