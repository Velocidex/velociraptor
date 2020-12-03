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
		ExistingDir()

	gui_command_no_browser = gui_command.Flag(
		"nobrowser", "Do not bring up the browser").Bool()

	gui_command_no_client = gui_command.Flag(
		"noclient", "Do not bring up a client").Bool()
)

func doGUI() {
	var err error

	datastore_directory := *gui_command_datastore
	if datastore_directory == "" {
		datastore_directory = os.TempDir()
	}

	datastore_directory, err = filepath.Abs(datastore_directory)
	kingpin.FatalIfError(err, "Unable find path.")

	server_config_path := filepath.Join(datastore_directory, "server.config.yaml")
	client_config_path := filepath.Join(datastore_directory, "client.config.yaml")

	// Try to open the config file from there
	config_obj, err := DefaultConfigLoader.
		WithVerbose(true).
		WithFileLoader(server_config_path).
		LoadAndValidate()
	if err != nil || config_obj.Frontend == nil {

		// Need to generate a new config. This config is not
		// really suitable for use in a proper deployment but
		// it is used here just to bring up the GUI and a self
		// client. It is useful for demonstration purposes and
		// to just be able to use the notebook and build an
		// offline collector.
		logging.Prelog("No valid config found - " +
			"will generare a new one at <green>" + server_config_path)

		config_obj = config.GetDefaultConfig()
		err := generateNewKeys(config_obj)
		kingpin.FatalIfError(err, "Unable to create config.")

		// GUI Configuration - hard coded username/password
		// and no SSL are suitable for local deployment only!
		config_obj.GUI.BindAddress = "127.0.0.1"
		config_obj.GUI.BindPort = 8889

		// Frontend only suitable for local client
		config_obj.Frontend.BindAddress = "127.0.0.1"
		config_obj.Frontend.BindPort = 8000

		// Client configuration.
		config_obj.Client.ServerUrls = []string{"https://localhost:8000/"}
		config_obj.Client.UseSelfSignedSsl = true

		write_back := filepath.Join(datastore_directory, "Velociraptor.writeback.yaml")
		config_obj.Client.WritebackWindows = write_back
		config_obj.Client.WritebackLinux = write_back
		config_obj.Client.WritebackDarwin = write_back

		// Do not use a local buffer file since there is no
		// point - we are by definition directly connected.
		config_obj.Client.LocalBuffer.DiskSize = 0
		config_obj.Client.LocalBuffer.FilenameWindows = ""
		config_obj.Client.LocalBuffer.FilenameLinux = ""
		config_obj.Client.LocalBuffer.FilenameDarwin = ""

		// Make the client use the datastore_directory for tempfiles as well.
		tmpdir := filepath.Join(datastore_directory, "temp")
		err = os.MkdirAll(tmpdir, 0700)
		kingpin.FatalIfError(err, "Unable to create temp directory.")

		config_obj.Client.TempdirLinux = tmpdir
		config_obj.Client.TempdirWindows = tmpdir
		config_obj.Client.TempdirDarwin = tmpdir

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

		fd, err := os.OpenFile(server_config_path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		kingpin.FatalIfError(err, "Open file %s", server_config_path)
		_, err = fd.Write(serialized)
		kingpin.FatalIfError(err, "Write file %s", server_config_path)
		fd.Close()

		// Now also write a client config
		client_config := getClientConfig(config_obj)
		client_config.Logging = config_obj.Logging

		serialized, err = yaml.Marshal(client_config)
		kingpin.FatalIfError(err, "Unable to create config.")

		fd, err = os.OpenFile(client_config_path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		kingpin.FatalIfError(err, "Open file %s", client_config_path)
		_, err = fd.Write(serialized)
		kingpin.FatalIfError(err, "Write file %s", client_config_path)
		fd.Close()

	}

	// Now start the frontend
	ctx, cancel := install_sig_handler()
	defer cancel()

	sm := services.NewServiceManager(ctx, config_obj)
	defer sm.Close()

	server, err := startFrontend(sm)
	kingpin.FatalIfError(err, "startFrontend")
	defer server.Close()

	// Just try to open the browser in the background.
	if !*gui_command_no_browser {
		go func() {
			url := fmt.Sprintf("https://admin:password@%v:%v/app.html#/welcome",
				config_obj.GUI.BindAddress,
				config_obj.GUI.BindPort)
			res := OpenBrowser(url)
			if !res {
				logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
				logger.Error(
					"Failed to open browser... you can try to connect directly to %v",
					url)
			}
		}()
	}

	if !*gui_command_no_client {
		*verbose_flag = true
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("Running client from %v", client_config_path)
		go RunClient(ctx, sm.Wg, &client_config_path)
	}

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
