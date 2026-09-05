package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/config"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/startup"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
)

/*
   To inspect the produce rpm file:

   rpm2cpio ./velociraptor-server-0.76.5.x86_64.rpm | less

   rpm -qp --scripts velociraptor-server-0.77.2.x86_64.rpm  | less
*/

var (
	rpm_command = app.Command(
		"rpm", "Create an rpm package")

	rpm_command_release = rpm_command.Flag(
		"release", "Rpm package release version").Default("A").String()

	client_rpm_command = rpm_command.Command(
		"client", "Create a client package from a server config file.")

	server_rpm_command = rpm_command.Command(
		"server", "Create a server package from a server config file.")

	server_rpm_command_output = server_rpm_command.Flag(
		"output", "Directory to store rpms in. (Default current directory)").
		Default(".").String()

	server_rpm_command_binary = server_rpm_command.Flag(
		"binary", "The binary to package").String()

	server_rpm_command_user = server_rpm_command.Flag(
		"server_user", "The (existing) server user to run the packaged service as.").String()

	server_rpm_command_group = server_rpm_command.Flag(
		"server_group", "The (existing) server group to run the packaged service as.").String()

	client_rpm_command_output = client_rpm_command.Flag(
		"output", "Directory to store rpms in. (Default current directory)").
		Default(".").String()

	client_rpm_command_binary = client_rpm_command.Flag(
		"binary", "The binary to package").String()
)

// Use Systemd based start up scripts (Centos 7, 8) if /bin/systemctl exists on OS
// otherwise use Simple startup scripts for SysV-style init systems (Centos 6)
func doClientRPM() error {
	// Disable logging when creating a package - we may not create the
	// package on the same system where the logs should go.
	logging.DisableLogging()

	if *config_path == "" {
		return fmt.Errorf("A server config must be specified using the --config flag")
	}

	abs_config_path, err := filepath.Abs(*config_path)
	if err != nil {
		return err
	}

	temp_dir, err := tempfile.TempDir("debian")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp_dir)

	blank_config := config.GetDefaultConfig()
	blank_config.Datastore.Location = temp_dir
	blank_config.Datastore.FilestoreDirectory = temp_dir

	ctx, cancel := Install_sig_handler()
	defer cancel()

	sm, err := startup.StartToolServices(ctx, blank_config)
	if err != nil {
		return err
	}
	defer sm.Close()

	if *client_rpm_command_binary == "" {
		*client_rpm_command_binary, err = os.Executable()
		if err != nil {
			return err
		}
	}

	*client_rpm_command_binary, err = filepath.Abs(*client_rpm_command_binary)
	if err != nil {
		return err
	}

	// By default write to current directory
	if *client_rpm_command_output == "" {
		*client_rpm_command_output = "."
	}

	// By default it should be set to A
	if *rpm_command_release == "" {
		*rpm_command_release = "A"
	}

	logger := &LogWriter{config_obj: sm.Config}
	builder := services.ScopeBuilder{
		Config:     sm.Config,
		ACLManager: acl_managers.NewRoleACLManager(sm.Config, "administrator"),
		Logger:     log.New(logger, "", 0),
		Env: ordereddict.NewDict().
			Set("Release", *rpm_command_release).
			Set("Output", *client_rpm_command_output).
			Set("BinaryToPackage", *client_rpm_command_binary).
			Set("ConfigPath", abs_config_path),
	}

	query := `
       LET _ <= log(message="Packaging binary %v to client RPM", args=BinaryToPackage)

       SELECT OSPath
       FROM rpm_create(exe=BinaryToPackage,
                       directory_name=Output,
                       config=read_file(filename=ConfigPath, length=1000000),
                       release=Release)
`

	err = runQueryWithEnv(ctx, query, builder, "json")
	if err != nil {
		return err
	}

	return logger.Error
}

// Systemd based start up scripts (CentOS 7+)
func doServerRPM() error {
	// Disable logging when creating a package - we may not create the
	// package on the same system where the logs should go.
	logging.DisableLogging()

	if *config_path == "" {
		return fmt.Errorf("A server config must be specified using the --config flag")
	}

	abs_config_path, err := filepath.Abs(*config_path)
	if err != nil {
		return err
	}

	temp_dir, err := tempfile.TempDir("debian")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp_dir)

	blank_config := config.GetDefaultConfig()
	blank_config.Datastore.Location = temp_dir
	blank_config.Datastore.FilestoreDirectory = temp_dir

	ctx, cancel := Install_sig_handler()
	defer cancel()

	sm, err := startup.StartToolServices(ctx, blank_config)
	if err != nil {
		return err
	}
	defer sm.Close()

	if *server_rpm_command_binary == "" {
		*server_rpm_command_binary, err = os.Executable()
		if err != nil {
			return err
		}
	}

	*server_rpm_command_binary, err = filepath.Abs(*server_rpm_command_binary)
	if err != nil {
		return err
	}

	// By default write to current directory
	if *server_rpm_command_output == "" {
		*server_rpm_command_output = "."
	}

	// By default it should be set to A
	if *rpm_command_release == "" {
		*rpm_command_release = "A"
	}

	logger := &LogWriter{config_obj: sm.Config}
	builder := services.ScopeBuilder{
		Config:     sm.Config,
		ACLManager: acl_managers.NewRoleACLManager(sm.Config, "administrator"),
		Logger:     log.New(logger, "", 0),
		Env: ordereddict.NewDict().
			Set("Release", *rpm_command_release).
			Set("Output", *server_rpm_command_output).
			Set("BinaryToPackage", *server_rpm_command_binary).
			Set("ConfigPath", abs_config_path).
			Set("ServiceUser", *server_rpm_command_user).
			Set("ServiceGroup", *server_rpm_command_group),
	}

	query := fmt.Sprintf(`
          LET _ <= log(message="Packaging binary %%v to server RPM", args=BinaryToPackage)
          LET Config <= read_file(filename=ConfigPath, length=1000000)
          SELECT OSPath
          FROM rpm_create(exe=BinaryToPackage, server=TRUE,
                          directory_name=Output,
                          config=Config,
                          service_user=ServiceUser,
                          service_group=ServiceGroup,
                          release=Release)`)

	err = runQueryWithEnv(ctx, query, builder, "json")
	if err != nil {
		return err
	}

	return logger.Error
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case client_rpm_command.FullCommand():
			FatalIfError(client_rpm_command, doClientRPM)

		case server_rpm_command.FullCommand():
			FatalIfError(server_rpm_command, doServerRPM)

		default:
			return false
		}
		return true
	})
}
