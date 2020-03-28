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
	"bytes"
	"compress/zlib"
	"io"
	"io/ioutil"
	"os"
	"runtime"

	"github.com/Velocidex/yaml"
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
func WritebackLocation(self *config_proto.Config) string {
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
func GetDefaultConfig() *config_proto.Config {
	result := &config_proto.Config{
		Version: &config_proto.Version{
			Name:      "velociraptor",
			Version:   constants.VERSION,
			BuildTime: build_time,
			Commit:    commit_hash,
		},
		Client: &config_proto.ClientConfig{
			WritebackDarwin: "/etc/velociraptor.writeback.yaml",
			WritebackLinux:  "/etc/velociraptor.writeback.yaml",
			WritebackWindows: "$ProgramFiles\\Velociraptor\\" +
				"velociraptor.writeback.yaml",
			MaxPoll: 60,

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
			BindAddress: "127.0.0.1",
			BindPort:    8889,
			InternalCidr: []string{
				"127.0.0.1/12", "192.168.0.0/16",
			},
			ReverseProxy: []*config_proto.ReverseProxyConfig{},
		},
		CA: &config_proto.CAConfig{},
		Frontend: &config_proto.FrontendConfig{
			// A public interface for clients to
			// connect to.
			BindAddress:   "0.0.0.0",
			BindPort:      8000,
			MaxUploadSize: constants.MAX_MEMORY * 2,
			DefaultClientMonitoringArtifacts: []string{
				// Essential for client resource telemetry.
				"Generic.Client.Stats",
			},
			ExpectedClients: 10000,
			PublicPath:      "/var/tmp/velociraptor/public",
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
	result.Client.Version = result.Version

	// On windows we need slightly different defaults.
	if runtime.GOOS == "windows" {
		result.Datastore.Location = "C:\\Windows\\Temp"
		result.Datastore.FilestoreDirectory = "C:\\Windows\\Temp"
		result.Client.LocalBuffer.Filename = "C:\\Windows\\Temp\\Velociraptor_Buffer.bin"
	}

	return result
}

func ReadEmbeddedConfig() (*config_proto.Config, error) {
	idx := bytes.IndexByte(FileConfigDefaultYaml, '\n')
	if FileConfigDefaultYaml[idx+1] == '#' {
		return nil, errors.New(
			"No embedded config - try to pack one with the pack command or " +
				"provide the --config flag.")
	}

	r, err := zlib.NewReader(bytes.NewReader(FileConfigDefaultYaml[idx+1:]))
	if err != nil {
		return nil, err
	}

	b := &bytes.Buffer{}
	io.Copy(b, r)
	r.Close()

	result := GetDefaultConfig()
	err = yaml.Unmarshal(b.Bytes(), result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// Load the config stored in the YAML file and returns a config object.
func LoadConfig(filename string) (*config_proto.Config, error) {
	default_config := GetDefaultConfig()
	result := GetDefaultConfig()

	verify_config := func(config_obj *config_proto.Config) {
		// TODO: Check if the config version is compatible with our
		// version. We always set the result's version to our version.
		config_obj.Version = default_config.Version
		config_obj.Client.Version = default_config.Version

		if config_obj.API.PinnedGwName == "" {
			config_obj.API.PinnedGwName = "GRPC_GW"
		}

		if config_obj.Client.PinnedServerName == "" {
			config_obj.Client.PinnedServerName = "VelociraptorServer"
		}
	}

	// If filename is specified we try to read from it.
	if filename != "" {
		data, err := ioutil.ReadFile(filename)
		if err == nil {
			err = yaml.UnmarshalStrict(data, result)
			if err != nil {
				return nil, errors.WithStack(err)
			}

			verify_config(result)

			return result, nil
		}
	}

	// Otherwise we try to read from the embedded config.
	embedded_config, err := ReadEmbeddedConfig()
	if err != nil {
		return nil, err
	}

	verify_config(embedded_config)

	return embedded_config, nil
}

func LoadClientConfig(filename string) (*config_proto.Config, error) {
	client_config, err := LoadConfig(filename)
	if err != nil {
		return nil, err
	}

	existing_writeback := &config_proto.Writeback{}
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
