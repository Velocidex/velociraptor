package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Velocidex/yaml/v2"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/users"
)

var (
	gui_command = app.Command(
		"gui", "Bring up a lazy GUI.")

	gui_command_datastore = gui_command.Flag(
		"datastore", "Path to a datastore directory (defaults to temp)").
		String()

	gui_command_no_browser = gui_command.Flag(
		"nobrowser", "Do not bring up the browser").Bool()
)

func doGUI() {
	var err error

	datastore_directory := *gui_command_datastore
	if datastore_directory == "" {
		datastore_directory = os.TempDir()
	}

	datastore_directory, err = filepath.Abs(datastore_directory)
	kingpin.FatalIfError(err, "Unable find path.")

	config_path := filepath.Join(datastore_directory, "server.config.yaml")

	// Try to open the config file from there
	config_obj, err := DefaultConfigLoader.
		WithVerbose(true).
		WithFileLoader(config_path).
		LoadAndValidate()
	if err != nil || config_obj.Frontend == nil {

		// Need to generate a new config.
		logging.Prelog("No valid config found - will generare a new one at <green>" +
			config_path)

		config_obj = config.GetDefaultConfig()
		err := generateNewKeys(config_obj)
		kingpin.FatalIfError(err, "Unable to create config.")

		config_obj.Client.ServerUrls = []string{"https://localhost:8000/"}
		config_obj.Datastore.Location = datastore_directory
		config_obj.Datastore.FilestoreDirectory = datastore_directory

		// Create a user with default password
		user_record, err := users.NewUserRecord("admin")
		kingpin.FatalIfError(err, "Unable to create user.")

		users.SetPassword(user_record, "password")
		config_obj.GUI.InitialUsers = append(config_obj.GUI.InitialUsers,
			&config_proto.GUIUser{
				Name:         user_record.Name,
				PasswordHash: hex.EncodeToString(user_record.PasswordHash),
				PasswordSalt: hex.EncodeToString(user_record.PasswordSalt),
			})

		// Write the config for next time
		serialized, err := yaml.Marshal(config_obj)
		kingpin.FatalIfError(err, "Unable to create config.")

		fd, err := os.OpenFile(config_path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		kingpin.FatalIfError(err, "Open file %s", config_path)
		_, err = fd.Write(serialized)
		kingpin.FatalIfError(err, "Write file %s", config_path)
		fd.Close()
	}

	// Now start the frontend
	ctx, cancel := install_sig_handler()
	defer cancel()

	sm := services.NewServiceManager(ctx, config_obj)
	defer sm.Close()

	server, err := startFrontend(sm, config_obj)
	kingpin.FatalIfError(err, "startFrontend")
	defer server.Close()

	// Just try to open the browser in the background.
	go func() {
		if *gui_command_no_browser {
			return
		}

		url := fmt.Sprintf("https://admin:password@%v:%v/app.html#/welcome",
			config_obj.GUI.BindAddress,
			config_obj.GUI.BindPort)
		res := OpenBrowser(url)
		if !res {
			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Error(fmt.Sprintf(
				"Failed to open browser... you can try to connect directory to %v",
				url))
		}
	}()

	sm.Wg.Wait()
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case gui_command.FullCommand():
			doGUI()
			return true
		}
		return false
	})
}
