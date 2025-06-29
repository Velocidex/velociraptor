package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/startup"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/vfilter"
)

var (
	deaddisk_command = app.Command(
		"deaddisk", "Create a deaddisk configuration")

	deaddisk_command_output = deaddisk_command.Arg(
		"output", "Output file to write config on").
		Required().OpenFile(os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)

	deaddisk_command_hostname = deaddisk_command.Flag("hostname",
		"The hostname to impersonate").Default("Virtual Host").String()

	deaddisk_command_add_windows_disk = deaddisk_command.Flag(
		"add_windows_disk", "Add a Windows Hard Disk Image").String()

	deaddisk_command_add_windows_directory = deaddisk_command.Flag(
		"add_windows_directory", "Add a Windows mounted directory").String()
)

func doDeadDisk() error {
	config_obj, err := APIConfigLoader.WithNullLoader().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	sm, err := startup.StartToolServices(ctx, config_obj)
	defer sm.Close()

	if err != nil {
		return err
	}

	// Close the file so we can overwrite it with VQL
	(*deaddisk_command_output).Close()

	image_path := *deaddisk_command_add_windows_disk
	if image_path == "" {
		image_path = *deaddisk_command_add_windows_directory
	}
	if image_path == "" {
		return fmt.Errorf("Either --add_windows_disk or --add_windows_directory should be specified.")
	}

	image_path, err = filepath.Abs(image_path)
	if err != nil {
		return err
	}

	logger := &LogWriter{config_obj: config_obj}
	builder := services.ScopeBuilder{
		Config:     sm.Config,
		ACLManager: acl_managers.NewRoleACLManager(sm.Config, "administrator"),
		Logger:     log.New(logger, "", 0),
		Env: ordereddict.NewDict().
			Set("ImagePath", image_path).
			Set("Output", (*deaddisk_command_output).Name()).
			Set("Hostname", *deaddisk_command_hostname).
			Set("Hostname", *deaddisk_command_hostname),
	}

	query := `
       SELECT copy(accessor="data",
                   filename=Remapping,
                   dest=Output) AS Remapping
       FROM Artifact.Generic.Utils.DeadDiskRemapping(
         ImagePath=ImagePath, Upload="N", Hostname=Hostname)
    `

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return err
	}
	scope := manager.BuildScope(builder)
	defer scope.Close()

	statements, err := vfilter.MultiParse(query)
	if err != nil {
		return err
	}

	out_fd := os.Stdout
	for _, vql := range statements {
		err = outputJSON(ctx, scope, vql, out_fd)
		if err != nil {
			return err
		}
	}

	return logger.Error
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case deaddisk_command.FullCommand():
			FatalIfError(deaddisk_command, doDeadDisk)

		default:
			return false
		}
		return true
	})
}
