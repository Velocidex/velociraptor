package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Velocidex/yaml/v2"
	errors "github.com/go-errors/errors"
	proto "google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/users"
	"www.velocidex.com/golang/velociraptor/services/writeback"
	"www.velocidex.com/golang/velociraptor/startup"
	vsurvey "www.velocidex.com/golang/velociraptor/tools/survey"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
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

func generateGUIConfig(datastore_directory, server_config_path, client_config_path string) (
	*config_proto.Config, error) {
	config_obj := config.GetDefaultConfig()
	err := vsurvey.GenerateNewKeys(config_obj)
	if err != nil {
		return nil, fmt.Errorf("Unable to create config: %w", err)
	}

	// GUI Configuration - hard coded username/password
	// and no SSL are suitable for local deployment only!
	config_obj.GUI.BindAddress = "127.0.0.1"
	config_obj.GUI.BindPort = 8889

	// Frontend only suitable for local client
	config_obj.Frontend.BindAddress = "127.0.0.1"
	config_obj.Frontend.BindPort = 8000
	config_obj.Frontend.DoNotCompressArtifacts = true

	// Client configuration.
	config_obj.Client.ServerUrls = []string{"wss://localhost:8000/"}
	config_obj.Client.UseSelfSignedSsl = true

	write_back := filepath.Join(datastore_directory, "Velociraptor.writeback.%NONCE%.yaml")
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
	if err != nil {
		return nil, fmt.Errorf("Unable to create temp directory: %w", err)
	}

	config_obj.Client.TempdirLinux = tmpdir
	config_obj.Client.TempdirWindows = tmpdir
	config_obj.Client.TempdirDarwin = tmpdir

	config_obj.Datastore.Location = datastore_directory
	config_obj.Datastore.FilestoreDirectory = datastore_directory

	// Make events run much faster in this configuration
	config_obj.Defaults.EventMaxWait = 1
	config_obj.Defaults.EventMaxWaitJitter = 1
	config_obj.Defaults.EventChangeNotifyAllClients = true

	// Load the "fs" accessor this time (It will be loaded
	// automatically after restart).
	err = initFilestoreAccessor(config_obj)
	if err != nil {
		return nil, err
	}

	// Create a user with default password
	user_record, err := users.NewUserRecord(config_obj, "admin")
	if err != nil {
		return nil, fmt.Errorf("Unable to create admin user: %w", err)
	}

	users.SetPassword(user_record, "password")
	config_obj.GUI.InitialUsers = append(config_obj.GUI.InitialUsers,
		&config_proto.GUIUser{
			Name:         user_record.Name,
			PasswordHash: hex.EncodeToString(user_record.PasswordHash),
			PasswordSalt: hex.EncodeToString(user_record.PasswordSalt),
		})

	// For the GUI org create a separate org.
	config_obj.GUI.InitialOrgs = append(config_obj.GUI.InitialOrgs,
		&config_proto.InitialOrgRecord{
			OrgId: "O123",
			Name:  "ACME Inc",
			Nonce: "ACME",
		})

	// Write the config for next time
	serialized, err := yaml.Marshal(config_obj)
	if err != nil {
		return nil, err
	}

	fd, err := os.OpenFile(server_config_path,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, fmt.Errorf("Open file %s: %w", server_config_path, err)
	}
	_, err = fd.Write(serialized)
	if err != nil {
		return nil, fmt.Errorf("Write file %s: %w", server_config_path, err)
	}
	fd.Close()

	// Now also write a client config
	client_config := config.GetClientConfig(config_obj)
	client_config.Logging = config_obj.Logging

	serialized, err = yaml.Marshal(client_config)
	if err != nil {
		return nil, err
	}

	fd, err = os.OpenFile(client_config_path,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, fmt.Errorf("Open file %s: %w", client_config_path, err)
	}
	_, err = fd.Write(serialized)
	if err != nil {
		return nil, fmt.Errorf("Write file %s: %w", client_config_path, err)
	}
	fd.Close()

	// Re-read the config from the file we just made.
	return makeDefaultConfigLoader().
		WithVerbose(true).
		WithFileLoader(server_config_path).LoadAndValidate()
}

func doGUI() error {
	// Start from a clean slate
	os.Setenv("VELOCIRAPTOR_CONFIG", "")
	os.Setenv("VELOCIRAPTOR_LITERAL_CONFIG", "")

	datastore_directory := *gui_command_datastore
	if datastore_directory == "" {
		datastore_directory = filepath.Join(
			tempfile.GetTempDir(), "gui_datastore")
		// Ensure the directory exists
		err := os.MkdirAll(datastore_directory, 0o777)
		if err != nil {
			return fmt.Errorf("Unable to create datastore directory: %w", err)
		}
	}

	datastore_directory, err := filepath.Abs(datastore_directory)
	if err != nil {
		return fmt.Errorf("Unable find path: %w", err)
	}

	server_config_path := filepath.Join(datastore_directory, "server.config.yaml")
	client_config_path := filepath.Join(datastore_directory, "client.config.yaml")

	// Try to open the config file from there
	config_obj, err := makeDefaultConfigLoader().
		WithVerbose(true).
		WithFileLoader(server_config_path).LoadAndValidate()
	if err != nil || config_obj.Frontend == nil {
		// Stop on hard errors but if the file does not exist we need
		// to create it below..
		hard_err, ok := err.(config.HardError)
		if ok && !errors.Is(hard_err.Err, os.ErrNotExist) {
			return err
		}

		// Need to generate a new config. This config is not
		// really suitable for use in a proper deployment but
		// it is used here just to bring up the GUI and a self
		// client. It is useful for demonstration purposes and
		// to just be able to use the notebook and build an
		// offline collector.
		logging.Prelog("No valid config found - "+
			"will generate a new one at <green> %s </>", server_config_path)

		config_obj, err = generateGUIConfig(
			datastore_directory, server_config_path, client_config_path)
		if err != nil {
			return err
		}
	}

	if config_obj.Services == nil {
		config_obj.Services = services.AllServerServicesSpec()
	}

	// Now start the frontend
	ctx, cancel := install_sig_handler()
	defer cancel()

	// Now start the frontend services
	sm, err := startup.StartFrontendServices(ctx, config_obj)
	if err != nil {
		return fmt.Errorf("starting frontend: %w", err)
	}
	defer sm.Close()

	// Just try to open the browser in the background.
	if !*gui_command_no_browser {
		go func() {
			// Recent chrome browsers do not forward the auth dialog
			// when the URL contains passwords. Therefore we can only
			// add the hard coded passwords if the actual password is
			// set to that.
			user_manager := services.GetUserManager()
			creds := ""
			ok, _ := user_manager.VerifyPassword(sm.Ctx, "admin", "admin", "password")
			if ok {
				creds = "admin:password"
			}

			url := fmt.Sprintf("https://%v@%v:%v/", creds,
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

		// Include the writeback in the client's configuration.
		config_obj, err := makeDefaultConfigLoader().
			WithRequiredClient().
			WithRequiredLogging().
			WithFileLoader(client_config_path).
			WithWriteback().LoadAndValidate()
		if err != nil {
			return err
		}

		sm.Wg.Add(1)
		go func() {
			defer sm.Wg.Done()

			err := RunClient(ctx, config_obj)
			if err != nil {
				logger.Error("<red>RunClient</>: %v", err)
			}
		}()

		org_manager, err := services.GetOrgManager()
		if err != nil {
			return err
		}

		// Try to start a client in our own org - it may not exist but
		// this is not an error.
		org_config_obj, err := org_manager.GetOrgConfig("O123")
		if err == nil {
			org_client_config := &config_proto.Config{
				Version: proto.Clone(org_config_obj.Version).(*config_proto.Version),
				Client:  proto.Clone(org_config_obj.Client).(*config_proto.ClientConfig),
			}

			// Make sure the client writeback is initialized
			writeback_service := writeback.GetWritebackService()
			_ = writeback_service.LoadWriteback(org_config_obj)

			sm.Wg.Add(1)
			go func() {
				defer sm.Wg.Done()

				err := RunClient(ctx, org_client_config)
				if err != nil {
					logger.Error("<red>RunClient</>: %v", err)
				}
			}()
		}
	}

	sm.Wg.Wait()

	return nil
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case gui_command.FullCommand():
			FatalIfError(gui_command, doGUI)
			return true
		}
		return false
	})
}
