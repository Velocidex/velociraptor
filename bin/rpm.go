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
	"www.velocidex.com/golang/velociraptor/vql/tools/packaging"
)

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
		"server_user", "The existing server user to run the packaged service as.").String()

	server_rpm_command_group = server_rpm_command.Flag(
		"server_group", "The existing server group to run the packaged service as.").String()

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

	server_user, server_group, err := packaging.ValidateCustomServerUserGroup(
		*server_rpm_command_user, *server_rpm_command_group)
	if err != nil {
		return err
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
			Set("ConfigPath", abs_config_path),
	}

	query_preamble := ""
	package_spec := ""

	if server_user != "" {
		builder.Env.Set("ServerUser", server_user)
		builder.Env.Set("ServerGroup", server_group)

		query_preamble = `
       LET S <= SELECT Spec FROM rpm_create(show_spec=TRUE, server=TRUE)
		 LET EffectiveUser <= ServerUser || S[0].Spec.Expansion.ServerUser
		 LET EffectiveGroup <= ServerGroup || EffectiveUser
       LET R <= S[0].Spec + dict(
		 Expansion=S[0].Spec.Expansion + dict(ServerUser=EffectiveUser, ServerGroup=EffectiveGroup)
       )
`
		package_spec = `,
                       package_spec=R`
	}

	query := fmt.Sprintf(`
       LET _ <= log(message="Packaging binary %%v to server RPM", args=BinaryToPackage)%s
       SELECT OSPath
       FROM rpm_create(exe=BinaryToPackage, server=TRUE%s,
                       directory_name=Output,
                       config=read_file(filename=ConfigPath, length=1000000),
                       release=Release)
`, query_preamble, package_spec)

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
