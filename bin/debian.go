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
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

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
	"archive/tar"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"

	"github.com/Velocidex/yaml"
	"github.com/xor-gate/debpkg"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/constants"
)

var (
	debian_command = app.Command(
		"debian", "Create a debian package")

	server_debian_command = debian_command.Command(
		"server", "Create a server package from a server config file.")

	server_debian_command_output = server_debian_command.Flag(
		"output", "Filename to output").Default(
		fmt.Sprintf("velociraptor_%s_server.deb", constants.VERSION)).
		String()

	server_debian_command_with_monitoring = server_debian_command.Flag(
		"with_monitoring", "Also include Grafana and Prometheus").Bool()

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
ExecStart=%s --config %s frontend
User=velociraptor
Group=velociraptor

[Install]
WantedBy=multi-user.target
`

	server_launcher = `
#!/bin/bash

export VELOCIRAPTOR_CONFIG=/etc/velociraptor/server.config.yaml
/usr/local/bin/velociraptor.bin ${@}
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
ExecStart=%s --config %s client

[Install]
WantedBy=multi-user.target
`

	prometheus_service_definition = `
[Unit]
Description=Velociraptor prometheus service
After=syslog.target network.target

[Service]
Type=simple
Restart=always
RestartSec=120
LimitNOFILE=20000
Environment=LANG=en_US.UTF-8
ExecStart=/usr/share/velociraptor_server/%s --config.file /etc/velociraptor/prometheus_velo.yaml

[Install]
WantedBy=multi-user.target
`
	grafana_service_definition = `
[Unit]
Description=Velociraptor Grafana
After=syslog.target network.target

[Service]
Type=simple
Restart=always
RestartSec=120
LimitNOFILE=20000
Environment=LANG=en_US.UTF-8
ExecStart=/usr/share/velociraptor_server/%s
WorkingDirectory=/usr/share/velociraptor_server/%s

[Install]
WantedBy=multi-user.target
`
)

func doServerDeb() {
	config_obj, err := get_server_config(*config_path)
	kingpin.FatalIfError(err, "Unable to load config file")

	// Debian packages always use the "velociraptor" user.
	config_obj.Frontend.RunAsUser = "velociraptor"

	res, err := yaml.Marshal(config_obj)
	kingpin.FatalIfError(err, "marshal")

	input := *server_debian_command_binary

	if input == "" {
		input, err = os.Executable()
		kingpin.FatalIfError(err, "Unable to open executable")
	}

	fd, err := os.Open(input)
	kingpin.FatalIfError(err, "Unable to open executable")
	defer fd.Close()

	header := make([]byte, 4)
	fd.Read(header)
	if binary.LittleEndian.Uint32(header) != 0x464c457f {
		kingpin.Fatalf("Binary does not appear to be an " +
			"ELF binary. Please specify the linux binary " +
			"using the --binary flag.")
	}

	deb := debpkg.New()
	defer deb.Close()

	deb.SetName("velociraptor-server")
	deb.SetVersion(constants.VERSION)
	deb.SetArchitecture("amd64")
	deb.SetMaintainer("Velocidex Innovations")
	deb.SetMaintainerEmail("support@velocidex.com")
	deb.SetHomepage("https://www.velocidex.com/docs")
	deb.SetShortDescription("Velociraptor server deployment.")
	deb.SetDepends("libcap2-bin, systemd")

	config_path := "/etc/velociraptor/server.config.yaml"
	velociraptor_bin := "/usr/local/bin/velociraptor"

	deb.AddFileString(string(res), config_path)
	deb.AddFileString(fmt.Sprintf(
		server_service_definition, velociraptor_bin, config_path),
		"/etc/systemd/system/velociraptor_server.service")
	deb.AddFile(input, velociraptor_bin+".bin")
	deb.AddFileString(server_launcher, velociraptor_bin)

	// Just a simple bare bones deb.
	if !*server_debian_command_with_monitoring {
		filestore_path := config_obj.Datastore.Location
		deb.AddControlExtraString("postinst", fmt.Sprintf(`
if ! getent group velociraptor >/dev/null; then
   addgroup --system velociraptor
fi

if ! getent passwd velociraptor >/dev/null; then
   adduser --system --home /etc/velociraptor/ --no-create-home \
     --ingroup velociraptor velociraptor --shell /bin/false \
     --gecos "Velociraptor Server"
fi

# Make the filestore path accessible to the user.
mkdir -p '%s'
chown -R velociraptor:velociraptor '%s' /etc/velociraptor/
chmod -R go-r /etc/velociraptor/
chmod +x /usr/local/bin/velociraptor

setcap CAP_SYS_RESOURCE,CAP_NET_BIND_SERVICE=+eip /usr/local/bin/velociraptor
/bin/systemctl enable velociraptor_server
/bin/systemctl start velociraptor_server
`, filestore_path, filestore_path))

		deb.AddControlExtraString("prerm", `
/bin/systemctl disable velociraptor_server
/bin/systemctl stop velociraptor_server
`)
		err = deb.Write(*server_debian_command_output)
		kingpin.FatalIfError(err, "Deb write")

		return
	}

	// Copy the graphana and prometheus distributions into the deb
	// so we are self contained.
	err = include_package(
		"https://dl.grafana.com/oss/release/grafana-6.1.3.linux-amd64.tar.gz",
		"/usr/share/velociraptor_server/", deb)
	kingpin.FatalIfError(err, "Downloading Grafana")

	deb.AddFileString(fmt.Sprintf(grafana_service_definition,
		"grafana-6.1.3/bin/grafana-server", "grafana-6.1.3/"),
		"/etc/systemd/system/velociraptor_grafana.service")

	err = include_package(
		"https://github.com/prometheus/prometheus/releases/download/v2.9.0/prometheus-2.9.0.linux-amd64.tar.gz",
		"/usr/share/velociraptor_server/", deb)
	kingpin.FatalIfError(err, "Downloading Prometheus")

	deb.AddFileString(fmt.Sprintf(prometheus_service_definition, "prometheus-2.9.0.linux-amd64/prometheus"),
		"/etc/systemd/system/velociraptor_prometheus.service")

	deb.AddFileString(`
global:
  scrape_interval:     5s

scrape_configs:
  - job_name: 'velociraptor'
    static_configs:
      - targets: ['localhost:8003']
`, "/etc/velociraptor/prometheus_velo.yaml")

	deb.AddControlExtraString("postinst", `
/bin/chmod +x /usr/share/velociraptor_server/*/bin/grafana-server /usr/share/velociraptor_server/*/prometheus

/bin/systemctl enable velociraptor_server
/bin/systemctl start velociraptor_server

/bin/systemctl enable velociraptor_grafana
/bin/systemctl start velociraptor_grafana

/bin/systemctl enable velociraptor_prometheus
/bin/systemctl start velociraptor_prometheus

`)

	deb.AddControlExtraString("prerm", `
/bin/systemctl disable velociraptor_server
/bin/systemctl stop velociraptor_server

/bin/systemctl disable velociraptor_grafana
/bin/systemctl stop velociraptor_grafana

/bin/systemctl disable velociraptor_prometheus
/bin/systemctl stop velociraptor_prometheus

`)

	err = deb.Write(*server_debian_command_output)
	kingpin.FatalIfError(err, "Deb write")
}

func doClientDeb() {
	config_obj, err := config.LoadConfig(*config_path)
	kingpin.FatalIfError(err, "Unable to load config file")

	res, err := yaml.Marshal(getClientConfig(config_obj))
	kingpin.FatalIfError(err, "marshal")

	input := *client_debian_command_binary

	if input == "" {
		input, err = os.Executable()
		kingpin.FatalIfError(err, "Unable to open executable")
	}

	fd, err := os.Open(input)
	kingpin.FatalIfError(err, "Unable to open executable")
	defer fd.Close()

	header := make([]byte, 4)
	fd.Read(header)
	if binary.LittleEndian.Uint32(header) != 0x464c457f {
		kingpin.Fatalf("Binary does not appear to be an " +
			"ELF binary. Please specify the linux binary " +
			"using the --binary flag.")
	}

	deb := debpkg.New()
	defer deb.Close()

	deb.SetName("velociraptor-client")
	deb.SetVersion(constants.VERSION)
	deb.SetArchitecture("amd64")
	deb.SetMaintainer("Velocidex Innovations")
	deb.SetMaintainerEmail("support@velocidex.com")
	deb.SetHomepage("https://www.velocidex.com")
	deb.SetShortDescription("Velociraptor client package.")

	config_path := "/etc/velociraptor/client.config.yaml"
	velociraptor_bin := "/usr/local/bin/velociraptor_client"

	deb.AddFileString(string(res), config_path)
	deb.AddFileString(fmt.Sprintf(
		client_service_definition, velociraptor_bin, config_path),
		"/etc/systemd/system/velociraptor_client.service")
	deb.AddFile(input, velociraptor_bin)

	deb.AddControlExtraString("postinst", `
/bin/systemctl enable velociraptor_client
/bin/systemctl start velociraptor_client
`)

	deb.AddControlExtraString("prerm", `
/bin/systemctl disable velociraptor_client
/bin/systemctl stop velociraptor_client
`)
	err = deb.Write(*client_debian_command_output)
	kingpin.FatalIfError(err, "Deb write")
}

// Download a tar/gz and unpack it into the deb package.
func include_package(url, deb_path string, deb *debpkg.DebPkg) error {
	filename := path.Base(url)
	fd, err := os.Open(filename)
	if os.IsNotExist(err) {
		err = DownloadFile(filename, url)
		if err != nil {
			return err
		}

		fd, err = os.Open(filename)
	}

	if err != nil {
		return err
	}
	defer fd.Close()

	gzf, err := gzip.NewReader(fd)
	if err != nil {
		return err
	}

	tarReader := tar.NewReader(gzf)

	i := 0
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}

		if header.Typeflag == tar.TypeReg {
			// This is not ideal but debpkg does not
			// support streamed readers.
			buffer, err := ioutil.ReadAll(tarReader)
			if err != nil {
				return err
			}

			name := path.Join(deb_path, header.Name)

			fmt.Println("(", i, ")", "Name: ", name)
			deb.AddFileString(string(buffer), name)
		}

		i++
	}

	return nil
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case server_debian_command.FullCommand():
			doServerDeb()

		case client_debian_command.FullCommand():
			doClientDeb()

		default:
			return false
		}
		return true
	})
}
