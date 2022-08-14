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
	"encoding/binary"
	"fmt"
	"os"
	"strings"

	"github.com/Velocidex/yaml/v2"
	"github.com/xor-gate/debpkg"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/services"
)

var (
	debian_command = app.Command(
		"debian", "Create a debian package")

	debian_command_arch = debian_command.Flag("arch",
		"Specify the debian package architecture (e.g. ppcle, amd64)").
		Default("amd64").String()

	server_debian_command = debian_command.Command(
		"server", "Create a server package from a server config file.")

	server_debian_command_output = server_debian_command.Flag(
		"output", "Filename to output").String()

	server_debian_command_binary = server_debian_command.Flag(
		"binary", "The binary to package").String()

	client_debian_command = debian_command.Command(
		"client", "Create a client package from a client config file.")

	client_debian_command_output = client_debian_command.Flag(
		"output", "Filename to output").Default(
		fmt.Sprintf("velociraptor_%s_client.deb", constants.VERSION)).
		String()

	client_debian_command_binary = client_debian_command.Flag(
		"binary", "The binary to package").String()

	server_service_definition = `
[Unit]
Description=Velociraptor linux amd64
After=syslog.target network.target

[Service]
Type=simple
Restart=always
RestartSec=120
LimitNOFILE=20000
Environment=LANG=en_US.UTF-8
ExecStart=%s --config %s frontend %s
User=velociraptor
Group=velociraptor
CapabilityBoundingSet=CAP_SYS_RESOURCE CAP_NET_BIND_SERVICE
AmbientCapabilities=CAP_SYS_RESOURCE CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
`

	server_post_install_template = `
if ! getent group velociraptor >/dev/null; then
   addgroup --system velociraptor
fi

if ! getent passwd velociraptor >/dev/null; then
   adduser --system --home /etc/velociraptor/ --no-create-home \
     --ingroup velociraptor velociraptor --shell /bin/false \
     --gecos "Velociraptor Server"
fi

# Make the filestore path accessible to the user.
mkdir -p '%s'/config

# Only chown two levels of the filestore directory in case
# this is an upgrade and there are many files already there.
# otherwise chown -R takes too long.
chown velociraptor:velociraptor '%s' '%s'/*
chown velociraptor:velociraptor -R /etc/velociraptor/

# Lock down permissions on the config file.
chmod -R go-r /etc/velociraptor/
chmod o+x /usr/local/bin/velociraptor /usr/local/bin/velociraptor.bin

# Allow the server to bind to low ports and increase its fd limit.
setcap CAP_SYS_RESOURCE,CAP_NET_BIND_SERVICE=+eip /usr/local/bin/velociraptor.bin
/bin/systemctl enable velociraptor_server
/bin/systemctl start velociraptor_server
`

	server_launcher = `#!/bin/bash

export VELOCIRAPTOR_CONFIG=/etc/velociraptor/server.config.yaml
if ! [[ -r "$VELOCIRAPTOR_CONFIG" ]] ; then
    echo "'$VELOCIRAPTOR_CONFIG' is not readable, you will need to run this as the velociraptor user ('sudo -u velociraptor bash')."
else
    exec /usr/local/bin/velociraptor.bin "$@"
fi
`
	client_service_definition = `
[Unit]
Description=Velociraptor linux client
After=syslog.target network.target

[Service]
Type=simple
Restart=always
RestartSec=120
LimitNOFILE=20000
Environment=LANG=en_US.UTF-8
ExecStart=%s --config %s client --quiet

[Install]
WantedBy=multi-user.target
`
)

func doServerDeb() error {
	// Disable logging when creating a deb - we may not create the
	// deb on the same system where the logs should go.
	_ = config.ValidateClientConfig(&config_proto.Config{})

	config_obj, err := makeDefaultConfigLoader().
		WithRequiredFrontend().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	if len(config_obj.ExtraFrontends) == 0 {
		return doSingleServerDeb(config_obj, "", nil)
	}

	// Build the master node
	node_name := services.GetNodeName(config_obj.Frontend)
	err = doSingleServerDeb(config_obj, "_master_"+node_name, nil)
	if err != nil {
		return err
	}

	for _, fe := range config_obj.ExtraFrontends {
		node_name := services.GetNodeName(fe)
		err = doSingleServerDeb(config_obj, "_minion_"+node_name,
			[]string{"--minion", "--node", node_name})
		if err != nil {
			return err
		}
	}

	return nil
}

func doSingleServerDeb(
	config_obj *config_proto.Config,
	variant string, extra_args []string) error {
	// Debian packages always use the "velociraptor" user.
	config_obj.Frontend.RunAsUser = "velociraptor"
	config_obj.ServerType = "linux"

	res, err := yaml.Marshal(config_obj)
	if err != nil {
		return err
	}

	input := *server_debian_command_binary

	if input == "" {
		input, err = os.Executable()
		if err != nil {
			return fmt.Errorf("Unable to open executable: %w", err)
		}
	}

	fd, err := os.Open(input)
	if err != nil {
		return fmt.Errorf("Unable to open executable: %w", err)
	}
	defer fd.Close()

	header := make([]byte, 4)
	_, err = fd.Read(header)
	if err != nil {
		return fmt.Errorf("Unable to read header: %w", err)
	}

	if binary.LittleEndian.Uint32(header) != 0x464c457f {
		return fmt.Errorf("Binary does not appear to be an " +
			"ELF binary. Please specify the linux binary " +
			"using the --binary flag.")
	}

	deb := debpkg.New()
	defer deb.Close()

	deb.SetName("velociraptor-server")
	deb.SetVersion(constants.VERSION)
	deb.SetArchitecture(*debian_command_arch)
	deb.SetMaintainer("Velocidex Enterprises")
	deb.SetMaintainerEmail("support@velocidex.com")
	deb.SetHomepage("https://www.velocidex.com/docs")
	deb.SetShortDescription("Velociraptor server deployment.")
	deb.SetDepends("libcap2-bin, systemd")

	config_path := "/etc/velociraptor/server.config.yaml"
	velociraptor_bin := "/usr/local/bin/velociraptor"

	err = deb.AddFileString(string(res), config_path)
	if err != nil {
		return fmt.Errorf("Adding file: %w", err)
	}
	err = deb.AddFileString(fmt.Sprintf(
		server_service_definition, velociraptor_bin, config_path,
		strings.Join(extra_args, " ")),
		"/etc/systemd/system/velociraptor_server.service")
	if err != nil {
		return fmt.Errorf("Adding file: %w", err)
	}
	err = deb.AddFile(input, velociraptor_bin+".bin")
	if err != nil {
		return fmt.Errorf("Adding file: %w", err)
	}
	err = deb.AddFileString(server_launcher, velociraptor_bin)
	if err != nil {
		return fmt.Errorf("Adding file: %w", err)
	}

	filestore_path := config_obj.Datastore.Location
	err = deb.AddControlExtraString("postinst", fmt.Sprintf(
		server_post_install_template,
		filestore_path, filestore_path, filestore_path))
	if err != nil {
		return fmt.Errorf("Adding file: %w", err)
	}

	err = deb.AddControlExtraString("prerm", `
/bin/systemctl disable velociraptor_server
/bin/systemctl stop velociraptor_server
`)
	if err != nil {
		return fmt.Errorf("Adding file: %w", err)
	}

	output_file := fmt.Sprintf("velociraptor_%s_server%s.deb",
		constants.VERSION, variant)

	fmt.Printf("Creating a package for %v\n", output_file)

	if *server_debian_command_output != "" {
		output_file = fmt.Sprintf("%s%s.deb",
			strings.TrimSuffix(*server_debian_command_output, ".deb"),
			variant)
	}

	err = deb.Write(output_file)
	if err != nil {
		return fmt.Errorf("Deb write: %w", err)
	}
	return nil
}

func doClientDeb() error {
	// Disable logging when creating a deb - we may not create the
	// deb on the same system where the logs should go.
	_ = config.ValidateClientConfig(&config_proto.Config{})

	config_obj, err := makeDefaultConfigLoader().
		WithRequiredClient().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	res, err := yaml.Marshal(getClientConfig(config_obj))
	if err != nil {
		return err
	}

	input := *client_debian_command_binary

	if input == "" {
		input, err = os.Executable()
		if err != nil {
			return fmt.Errorf("Unable to open executable: %w", err)
		}
	}

	fd, err := os.Open(input)
	if err != nil {
		return fmt.Errorf("Unable to open executable: %w", err)
	}
	defer fd.Close()

	header := make([]byte, 4)
	_, err = fd.Read(header)
	if err != nil {
		return fmt.Errorf("Unable to open executable: %w", err)
	}

	if binary.LittleEndian.Uint32(header) != 0x464c457f {
		return fmt.Errorf("Binary does not appear to be an " +
			"ELF binary. Please specify the linux binary " +
			"using the --binary flag.")
	}

	deb := debpkg.New()
	defer deb.Close()

	deb.SetName("velociraptor-client")
	deb.SetVersion(constants.VERSION)
	deb.SetArchitecture(*debian_command_arch)
	deb.SetMaintainer("Velocidex Enterprises")
	deb.SetMaintainerEmail("support@velocidex.com")
	deb.SetHomepage("https://www.velocidex.com")
	deb.SetShortDescription("Velociraptor client package.")

	config_path := "/etc/velociraptor/client.config.yaml"
	velociraptor_bin := "/usr/local/bin/velociraptor_client"

	err = deb.AddFileString(string(res), config_path)
	if err != nil {
		return fmt.Errorf("Deb write: %w", err)
	}

	err = deb.AddFileString(fmt.Sprintf(
		client_service_definition, velociraptor_bin, config_path),
		"/etc/systemd/system/velociraptor_client.service")
	if err != nil {
		return fmt.Errorf("Deb write: %w", err)
	}

	err = deb.AddFile(input, velociraptor_bin)
	if err != nil {
		return fmt.Errorf("Deb write: %w", err)
	}

	err = deb.AddControlExtraString("postinst", fmt.Sprintf(`
# Lock down permissions on the config file.
chmod -R go-r /etc/velociraptor/
chmod o+x "%s"

/bin/systemctl enable velociraptor_client
/bin/systemctl start velociraptor_client
`, velociraptor_bin))
	if err != nil {
		return fmt.Errorf("Deb write: %w", err)
	}

	err = deb.AddControlExtraString("prerm", `
/bin/systemctl disable velociraptor_client
/bin/systemctl stop velociraptor_client
`)
	if err != nil {
		return fmt.Errorf("Deb write: %w", err)
	}

	err = deb.Write(*client_debian_command_output)
	if err != nil {
		return fmt.Errorf("Deb write: %w", err)
	}

	return nil
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
