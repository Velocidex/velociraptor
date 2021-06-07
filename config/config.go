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
package config

import (
	"io/ioutil"
	"os"
	"runtime"

	"github.com/Velocidex/yaml/v2"
	errors "github.com/pkg/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
)

// Embed build time constants into here for reporting client version.
// https://husobee.github.io/golang/compile/time/variables/2015/12/03/compile-time-const.html
var (
	build_time  string
	commit_hash string
)

// Return the location of the writeback file.
func WritebackLocation(self *config_proto.Config) (string, error) {
	if self.Client == nil {
		return "", errors.New("Client not configured")
	}

	switch runtime.GOOS {
	case "darwin":
		return os.ExpandEnv(self.Client.WritebackDarwin), nil
	case "linux":
		return os.ExpandEnv(self.Client.WritebackLinux), nil
	case "windows":
		return os.ExpandEnv(self.Client.WritebackWindows), nil
	default:
		return os.ExpandEnv(self.Client.WritebackLinux), nil
	}
}

func GetVersion() *config_proto.Version {
	return &config_proto.Version{
		Name:      "velociraptor",
		Version:   constants.VERSION,
		BuildTime: build_time,
		Commit:    commit_hash,
	}
}

// Create a default configuration object.
func GetDefaultConfig() *config_proto.Config {
	result := &config_proto.Config{
		Client: &config_proto.ClientConfig{
			WritebackDarwin: "/etc/velociraptor.writeback.yaml",
			WritebackLinux:  "/etc/velociraptor.writeback.yaml",
			WritebackWindows: "$ProgramFiles\\Velociraptor\\" +
				"velociraptor.writeback.yaml",
			TempdirWindows: "$ProgramFiles\\Velociraptor\\Tools",
			MaxPoll:        60,

			// Local ring buffer to queue messages to the
			// server. If the server is not available we
			// write these to disk so we can send them
			// next time we are online.
			LocalBuffer: &config_proto.RingBufferConfig{
				MemorySize:      50 * 1024 * 1024,
				DiskSize:        1024 * 1024 * 1024,
				FilenameLinux:   "/var/tmp/Velociraptor_Buffer.bin",
				FilenameWindows: "$TEMP/Velociraptor_Buffer.bin",
				FilenameDarwin:  "/var/tmp/Velociraptor_Buffer.bin",
			},

			// Specific instructions for the
			// windows service installer.
			WindowsInstaller: &config_proto.WindowsInstallerConfig{
				ServiceName: "Velociraptor",
				InstallPath: "$ProgramFiles\\Velociraptor\\" +
					"Velociraptor.exe",
				ServiceDescription: "Velociraptor service",
			},

			DarwinInstaller: &config_proto.DarwinInstallerConfig{
				ServiceName: "com.velocidex.velociraptor",
				InstallPath: "/usr/local/sbin/velociraptor",
			},

			// If set to true this will stop
			// arbitrary code execution on the
			// client.
			PreventExecve:    false,
			MaxUploadSize:    constants.MAX_MEMORY,
			PinnedServerName: "VelociraptorServer",
		},
		API: &config_proto.APIConfig{
			// Bind port for gRPC endpoint - this should not
			// normally be exposed.
			BindAddress:  "127.0.0.1",
			BindPort:     8001,
			BindScheme:   "tcp",
			PinnedGwName: "GRPC_GW",
		},
		GUI: &config_proto.GUIConfig{
			// Bind port for GUI. If you expose this on a
			// reachable IP address you must enable TLS!
			BindAddress:  "127.0.0.1",
			BindPort:     8889,
			ReverseProxy: []*config_proto.ReverseProxyConfig{},
			Authenticator: &config_proto.Authenticator{
				Type: "Basic",
			},
		},
		CA: &config_proto.CAConfig{},
		Frontend: &config_proto.FrontendConfig{
			Hostname: "localhost",

			// A public interface for clients to
			// connect to.
			BindAddress: "0.0.0.0",
			BindPort:    8000,
			DefaultClientMonitoringArtifacts: []string{
				// Essential for client resource telemetry.
				"Generic.Client.Stats",
			},
			DynDns: &config_proto.DynDNSConfig{},
			Resources: &config_proto.FrontendResourceControl{
				ExpectedClients:        30000, // Controls RSA cache size
				ConnectionsPerSecond:   100,   // QPS load shedding limit (>1000 disable)
				Concurrency:            0,     // By default 2 * CPU count
				TargetHeapSize:         0,     // (Disabled) Set to control concurrency to match target heap size.
				NotificationsPerSecond: 30,
				MaxUploadSize:          constants.MAX_MEMORY * 2,
			},
			GRPCPoolMaxSize: 100,
			GRPCPoolMaxWait: 60,
		},
		Datastore: &config_proto.DatastoreConfig{
			Implementation: "FileBaseDataStore",

			// Users would probably need to change
			// this to something more permanent.
			Location:           "/var/tmp/velociraptor",
			FilestoreDirectory: "/var/tmp/velociraptor",
		},
		Writeback: &config_proto.Writeback{},
		Mail:      &config_proto.MailConfig{},
		Logging:   &config_proto.LoggingConfig{},
		Monitoring: &config_proto.MonitoringConfig{
			BindAddress: "127.0.0.1",
			BindPort:    8003,
		},
		ApiConfig: &config_proto.ApiClientConfig{},
	}

	// The client's version needs to keep in sync with the
	// server's version.
	result.Client.Version = GetVersion()
	result.Version = result.Client.Version

	// On windows we need slightly different defaults.
	if runtime.GOOS == "windows" {
		result.Datastore.Location = "C:\\Windows\\Temp"
		result.Datastore.FilestoreDirectory = "C:\\Windows\\Temp"
	}

	return result
}

func WriteConfigToFile(filename string, config *config_proto.Config) error {
	bytes, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	// Make sure the new file is only readable by root.
	err = ioutil.WriteFile(filename, bytes, 0600)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// Update the client's writeback file.
func UpdateWriteback(config_obj *config_proto.Config) error {
	location, err := WritebackLocation(config_obj)
	if err != nil {
		return err
	}

	bytes, err := yaml.Marshal(config_obj.Writeback)
	if err != nil {
		return errors.WithStack(err)
	}

	// Make sure the new file is only readable by root.
	err = ioutil.WriteFile(location, bytes, 0600)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}
