//go:build !aix
// +build !aix

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
	"log"
	"os"
	"path/filepath"

	"github.com/Velocidex/ordereddict"
	errors "github.com/go-errors/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/startup"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/vfilter"
)

var (
	repack_command = config_command.Command(
		"repack", "Embed a configuration file inside the binary.")

	repack_command_exe = repack_command.Flag(
		"exe", "Use an alternative exe.").String()

	repack_command_msi = repack_command.Flag(
		"msi", "Use an msi to repack (synonym to --exe).").String()

	repack_command_config = repack_command.Arg(
		"config_file", "The filename to write into the binary.").
		Required().File()

	repack_command_binaries = repack_command.Flag(
		"binaries", "The list of tool names to append to the binary.").
		Strings()

	repack_command_output = repack_command.Arg(
		"output", "The filename to write the repacked binary.").
		Required().String()
)

func doRepack() error {
	logging.DisableLogging()

	executable := *repack_command_exe
	if executable == "" {
		executable = *repack_command_msi
	}

	if executable == "" {
		executable, _ = os.Executable()
	}

	// Make sure the executable path is an absolute file and we can
	// read it.
	if executable == "" {
		return errors.New("Unable to find executable to repack")
	}

	abs_executable, err := filepath.Abs(executable)
	if err != nil {
		return err
	}

	executable = abs_executable

	// Read the config file
	config_data, err := utils.ReadAllWithLimit(*repack_command_config,
		constants.MAX_MEMORY)
	if err != nil {
		return err
	}

	config_obj := &config_proto.Config{}
	if isConfigSpecified(os.Args) {
		config_obj, err = APIConfigLoader.WithNullLoader().
			LoadAndValidate()
		if err != nil {
			return err
		}
		config_obj.Services = services.GenericToolServices()
		if config_obj.Datastore != nil && config_obj.Datastore.Location != "" {
			config_obj.Services.IndexServer = true
			config_obj.Services.ClientInfo = true
			config_obj.Services.Label = true
		}

	} else {
		config_obj.Services = services.GenericToolServices()
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	sm, err := startup.StartToolServices(ctx, config_obj)
	defer sm.Close()

	if err != nil {
		return err
	}

	output_path, err := filepath.Abs(*repack_command_output)
	if err != nil {
		return err
	}
	logger := &LogWriter{config_obj: config_obj}
	builder := services.ScopeBuilder{
		Config:     sm.Config,
		ACLManager: acl_managers.NewRoleACLManager(sm.Config, "administrator"),
		Uploader: &uploads.FileBasedUploader{
			UploadDir: filepath.Dir(output_path),
		},
		Logger: log.New(logger, "", 0),
		Env: ordereddict.NewDict().
			Set("ConfigData", config_data).
			Set("Exe", executable).
			Set("Binaries", *repack_command_binaries).
			Set("UploadName", filepath.Base(output_path)),
	}

	query := `
       SELECT repack(exe=Exe, accessor="file",
          config=ConfigData, upload_name=UploadName) AS RepackInfo
       FROM scope()
`

	// Repacking binaries requires a working inventory service -
	// therefore a server config.
	if len(*repack_command_binaries) > 0 {
		query = `
       SELECT repack(exe=Exe, accessor="file", binaries=Binaries,
          config=ConfigData, upload_name=UploadName) AS RepackInfo
       FROM scope()
`
	}

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
		if command == repack_command.FullCommand() {
			FatalIfError(repack_command, doRepack)
			return true
		}

		return false
	})
}
