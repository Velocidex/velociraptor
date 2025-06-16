//go:build !aix
// +build !aix

// This command creates a customized server deb package which may be
// used to automatically deploy Velociraptor server (e.g. in a Docker
// container or on its own VM). The intention is to make this as self
// contained as possible to speed up real deployments. The normal way
// to use it is to:

// 1. Create a Velociraptor server config file (velociraptor config generate)
// 2. Edit it as needed.
// 3. build a deb package
// 4. Push the package to a new cloud vm instance. You will need to
//    configuration DNS etc separately.

// NOTE that this embeds the server config in the deb package.

// Invoke by:
// velociraptor --config server.config.yaml debian server

// Additionally the "debian client" command will create a similar deb
// package with the client configuration.

/*
Velociraptor - Dig Deeper
Copyright (C) 2019 Velocidex Enterprises.

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
	"log"
	"os"

	"github.com/Velocidex/ordereddict"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/startup"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
)

var (
	debian_command = app.Command(
		"debian", "Create a debian package")

	debian_command_release = debian_command.Flag("release",
		"Specify the debian release number").String()

	server_debian_command = debian_command.Command(
		"server", "Create a server package from a server config file.")

	server_debian_command_output = server_debian_command.Flag(
		"output", "Directory to store deb files in. (Default current directory)").
		Default(".").String()

	server_debian_command_binary = server_debian_command.Flag(
		"binary", "The binary to package").String()

	client_debian_command = debian_command.Command(
		"client", "Create a client package from a client config file.")

	client_debian_command_output = client_debian_command.Flag(
		"output", "Directory to store deb package in. (Default current directory)").
		Default(".").String()

	client_debian_command_binary = client_debian_command.Flag(
		"binary", "The binary to package").String()
)

func doServerDeb() error {
	// Disable logging when creating a deb - we may not create the
	// deb on the same system where the logs should go.
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().
		WithRequiredFrontend().LoadAndValidate()
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

	if *server_debian_command_binary == "" {
		*server_debian_command_binary, err = os.Executable()
		if err != nil {
			return err
		}
	}

	// By default write to current directory
	if *server_debian_command_output == "" {
		*server_debian_command_output = "."
	}

	logger := &LogWriter{config_obj: sm.Config}
	builder := services.ScopeBuilder{
		Config:     sm.Config,
		ACLManager: acl_managers.NewRoleACLManager(sm.Config, "administrator"),
		Logger:     log.New(logger, "", 0),
		Env: ordereddict.NewDict().
			Set("Release", *debian_command_release).
			Set("Output", *server_debian_command_output).
			Set("BinaryToPackage", *server_debian_command_binary),
	}

	query := `
       LET _ <= log(message="Packaging binary %v to server Deb", args=BinaryToPackage)

       SELECT OSPath
       FROM deb_create(exe=BinaryToPackage, server=TRUE,
                       directory_name=Output,
                       release=Release)`

	return runQueryWithEnv(query, builder, "json")
}

func doClientDeb() error {
	// Disable logging when creating a deb - we may not create the
	// deb on the same system where the logs should go.
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().
		WithRequiredClient().LoadAndValidate()
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

	if *client_debian_command_binary == "" {
		*client_debian_command_binary, err = os.Executable()
		if err != nil {
			return err
		}
	}

	// By default write to current directory
	if *client_debian_command_output == "" {
		*client_debian_command_output = "."
	}

	logger := &LogWriter{config_obj: sm.Config}
	builder := services.ScopeBuilder{
		Config:     sm.Config,
		ACLManager: acl_managers.NewRoleACLManager(sm.Config, "administrator"),
		Logger:     log.New(logger, "", 0),
		Env: ordereddict.NewDict().
			Set("Release", *debian_command_release).
			Set("Output", *client_debian_command_output).
			Set("BinaryToPackage", *client_debian_command_binary),
	}

	query := `
       LET _ <= log(message="Packaging binary %v to client Deb", args=BinaryToPackage)

       SELECT OSPath
       FROM deb_create(exe=BinaryToPackage,
                       directory_name=Output,
                       release=Release)`

	return runQueryWithEnv(query, builder, "json")
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case server_debian_command.FullCommand():
			FatalIfError(server_debian_command, doServerDeb)

		case client_debian_command.FullCommand():
			FatalIfError(client_debian_command, doClientDeb)

		default:
			return false
		}
		return true
	})
}
