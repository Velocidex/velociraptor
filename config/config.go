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

	"github.com/Velocidex/yaml"
	errors "github.com/pkg/errors"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
)

// Embed build time constants into here for reporting client version.
// https://husobee.github.io/golang/compile/time/variables/2015/12/03/compile-time-const.html
var (
	build_time  string
	commit_hash string
)

// Return the location of the writeback file.
func WritebackLocation(self *api_proto.Config) string {
	switch runtime.GOOS {
	case "darwin":
		return os.ExpandEnv(self.Client.WritebackDarwin)
	case "linux":
		return os.ExpandEnv(self.Client.WritebackLinux)
	case "windows":
		return os.ExpandEnv(self.Client.WritebackWindows)
	default:
		return os.ExpandEnv(self.Client.WritebackLinux)
	}
}

// Create a default configuration object.
func GetDefaultConfig() *api_proto.Config {
	return &api_proto.Config{
		Version: &api_proto.Version{
			Name:      "velociraptor",
			Version:   constants.VERSION,
			BuildTime: build_time,
			Commit:    commit_hash,
		},
		Client: &api_proto.ClientConfig{
			WritebackDarwin: "/etc/velociraptor.writeback.yaml",
			WritebackLinux:  "/etc/velociraptor.writeback.yaml",
			WritebackWindows: "$ProgramFiles\\Velociraptor\\" +
				"velociraptor.writeback.yaml",
			MaxPoll: 600,

			// Specific instructions for the
			// windows service installer.
			WindowsInstaller: &api_proto.WindowsInstallerConfig{
				ServiceName: "Velociraptor",
				InstallPath: "$ProgramFiles\\Velociraptor\\" +
					"Velociraptor.exe",
				ServiceDescription: "Velociraptor service",
			},

			DarwinInstaller: &api_proto.DarwinInstallerConfig{
				ServiceName: "com.velocidex.velociraptor",
				InstallPath: "/usr/local/sbin/velociraptor",
			},

			// If set to true this will stop
			// arbitrary code execution on the
			// client.
			PreventExecve: false,
		},
		API: &api_proto.APIConfig{
			// Bind port for gRPC endpoint - this should not
			// normally be exposed.
			BindAddress: "127.0.0.1",
			BindPort:    8001,
			BindScheme:  "tcp",
		},
		GUI: &api_proto.GUIConfig{
			// Bind port for GUI. If you expose this on a
			// reachable IP address you must enable TLS!
			BindAddress: "127.0.0.1",
			BindPort:    8889,
			InternalCidr: []string{
				"127.0.0.1/12", "192.168.0.0/16",
			},
		},
		CA: &api_proto.CAConfig{},
		Frontend: &api_proto.FrontendConfig{
			// A public interface for clients to
			// connect to.
			BindAddress: "0.0.0.0",
			BindPort:    8000,
		},
		Datastore: &api_proto.DatastoreConfig{
			Implementation: "FileBaseDataStore",

			// Users would probably need to change
			// this to something more permanent.
			Location:           "/tmp/velociraptor",
			FilestoreDirectory: "/tmp/velociraptor",
		},
		Flows:     &api_proto.FlowsConfig{},
		Writeback: &api_proto.Writeback{},
		Events: &api_proto.ClientEvents{
			Artifacts: []string{"Generic.Client.Stats"},
			Version:   1,
		},
		Mail:    &api_proto.MailConfig{},
		Logging: &api_proto.LoggingConfig{},
		Monitoring: &api_proto.MonitoringConfig{
			BindAddress: "127.0.0.1",
			BindPort:    8003,
		},
	}
}

func maybeReadEmbeddedConfig() *api_proto.Config {
	result := GetDefaultConfig()
	err := yaml.Unmarshal(FileConfigDefaultYaml, result)
	if err != nil {
		return nil
	}

	return result
}

// Load the config stored in the YAML file and returns a config object.
func LoadConfig(filename string) (*api_proto.Config, error) {
	default_config := GetDefaultConfig()
	result := GetDefaultConfig()

	// If filename is specified we try to read from it.
	if filename != "" {
		data, err := ioutil.ReadFile(filename)
		if err == nil {
			err = yaml.UnmarshalStrict(data, result)
			if err != nil {
				return nil, errors.WithStack(err)
			}

			// TODO: Check if the config version is compatible with our
			// version. We always set the result's version to our version.
			result.Version = default_config.Version
			result.Client.Version = default_config.Version

			return result, nil

		}
	}

	// Otherwise we try to read from the embedded config.
	embedded_config := maybeReadEmbeddedConfig()
	if embedded_config == nil {
		return nil, errors.New(
			"No embedded config - try to pack one with the pack command or " +
				"provide the --config flag.")
	}

	embedded_config.Version = default_config.Version
	return embedded_config, nil
}

func LoadClientConfig(filename string) (*api_proto.Config, error) {
	client_config, err := LoadConfig(filename)
	if err != nil {
		return nil, err
	}

	existing_writeback := &api_proto.Writeback{}
	data, err := ioutil.ReadFile(WritebackLocation(client_config))
	// Failing to read the file is not an error - the file may not
	// exist yet.
	if err == nil {
		err = yaml.Unmarshal(data, existing_writeback)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	// Merge the writeback with the config.
	client_config.Writeback = existing_writeback
	return client_config, nil
}

func WriteConfigToFile(filename string, config *api_proto.Config) error {
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
func UpdateWriteback(config_obj *api_proto.Config) error {
	if WritebackLocation(config_obj) == "" {
		return nil
	}

	bytes, err := yaml.Marshal(config_obj.Writeback)
	if err != nil {
		return errors.WithStack(err)
	}

	// Make sure the new file is only readable by root.
	err = ioutil.WriteFile(WritebackLocation(config_obj), bytes, 0600)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}
