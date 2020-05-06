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
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"regexp"
	"runtime"
	"strings"

	"github.com/Velocidex/yaml/v2"
	errors "github.com/pkg/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/logging"
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
			DynDns:          &config_proto.DynDNSConfig{},
			ExpectedClients: 10000,
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

// Loads the config from a file, or possibly from embedded.
func LoadConfig(filename string) (*config_proto.Config, error) {
	result := &config_proto.Config{}

	// If filename is specified we try to read from it.
	if filename != "" {
		data, err := ioutil.ReadFile(filename)
		if err == nil {
			err = yaml.UnmarshalStrict(data, result)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			return result, nil
		}
	}

	// Otherwise we try to read from the embedded config.
	embedded_config, err := ReadEmbeddedConfig()
	if err != nil {
		return nil, err
	}

	return embedded_config, nil
}

// Ensures client config is valid, fills in defaults for missing values etc.
func ValidateClientConfig(config_obj *config_proto.Config) error {
	migrate(config_obj)

	// Ensure we have all the sections that are required.
	if config_obj.Client == nil {
		return errors.New("No Client config")
	}

	if config_obj.Client.CaCertificate == "" {
		return errors.New("No Client.ca_certificate configured")
	}

	if config_obj.Client.Nonce == "" {
		return errors.New("No Client.nonce configured")
	}

	if config_obj.Client.ServerUrls == nil {
		return errors.New("No Client.server_urls configured")
	}

	if WritebackLocation(config_obj) == "" {
		return errors.New("No writeback location specified.")
	}

	// Add defaults
	if config_obj.Logging == nil {
		config_obj.Logging = &config_proto.LoggingConfig{}
	}

	// If no local buffer is specified make an in memory one.
	if config_obj.Client.LocalBuffer == nil {
		config_obj.Client.LocalBuffer = &config_proto.RingBufferConfig{
			MemorySize: 50 * 1024 * 1024,
		}
	}

	if config_obj.Client.MaxPoll == 0 {
		config_obj.Client.MaxPoll = 60
	}

	if config_obj.Client.PinnedServerName == "" {
		config_obj.Client.PinnedServerName = "VelociraptorServer"
	}

	if config_obj.Client.MaxUploadSize == 0 {
		config_obj.Client.MaxUploadSize = 5242880
	}

	config_obj.Version = GetVersion()
	config_obj.Client.Version = config_obj.Version

	for _, url := range config_obj.Client.ServerUrls {
		if !strings.HasSuffix(url, "/") {
			return errors.New(
				"Configuration Client.server_urls must end with /")
		}
	}

	return logging.InitLogging(config_obj)
}

// Ensures server config is valid, fills in defaults for missing values etc.
func ValidateFrontendConfig(config_obj *config_proto.Config) error {
	// Check for older version.
	migrate(config_obj)

	err := ValidateClientConfig(config_obj)
	if err != nil {
		return err
	}

	if config_obj.API == nil {
		return errors.New("No API config")
	}
	if config_obj.GUI == nil {
		return errors.New("No GUI config")
	}
	if config_obj.GUI.GwCertificate == "" {
		return errors.New("No GUI.gw_certificate config")
	}
	if config_obj.GUI.GwPrivateKey == "" {
		return errors.New("No GUI.gw_private_key config")
	}
	if config_obj.Frontend == nil {
		return errors.New("No Frontend config")
	}
	if config_obj.Frontend.Hostname == "" {
		return errors.New("No Frontend.hostname config")
	}
	if config_obj.Frontend.Certificate == "" {
		return errors.New("No Frontend.certificate config")
	}
	if config_obj.Frontend.PrivateKey == "" {
		return errors.New("No Frontend.private_key config")
	}
	if config_obj.Datastore == nil {
		return errors.New("No Datastore config")
	}

	// Fill defaults for optional sections
	if config_obj.Writeback == nil {
		config_obj.Writeback = &config_proto.Writeback{}
	}
	if config_obj.CA == nil {
		config_obj.CA = &config_proto.CAConfig{}
	}
	if config_obj.Frontend.ExpectedClients == 0 {
		config_obj.Frontend.ExpectedClients = 10000
	}
	if config_obj.Mail == nil {
		config_obj.Mail = &config_proto.MailConfig{}
	}
	if config_obj.Monitoring == nil {
		config_obj.Monitoring = &config_proto.MonitoringConfig{}
	}

	if config_obj.ApiConfig == nil {
		config_obj.ApiConfig = &config_proto.ApiClientConfig{}
	}

	if config_obj.API.PinnedGwName == "" {
		config_obj.API.PinnedGwName = "GRPC_GW"
	}

	// If mysql connection params are specified we create
	// a mysql_connection_string
	if config_obj.Datastore.MysqlConnectionString == "" &&
		(config_obj.Datastore.MysqlDatabase != "" ||
			config_obj.Datastore.MysqlServer != "" ||
			config_obj.Datastore.MysqlUsername != "" ||
			config_obj.Datastore.MysqlPassword != "") {
		config_obj.Datastore.MysqlConnectionString = fmt.Sprintf(
			"%s:%s@tcp(%s)/%s",
			config_obj.Datastore.MysqlUsername,
			config_obj.Datastore.MysqlPassword,
			config_obj.Datastore.MysqlServer,
			config_obj.Datastore.MysqlDatabase)
	}

	// On windows we require file locations to include a drive
	// letter.
	if config_obj.ServerType == "windows" {
		path_regex := regexp.MustCompile("^[a-zA-Z]:")
		path_check := func(parameter, value string) error {
			if !path_regex.MatchString(value) {
				return errors.New(fmt.Sprintf(
					"%s must have a drive letter.",
					parameter))
			}
			if strings.Contains(value, "/") {
				return errors.New(fmt.Sprintf(
					"%s can not contain / path separators on windows.",
					parameter))
			}
			return nil
		}

		err := path_check("Datastore.Locations",
			config_obj.Datastore.Location)
		if err != nil {
			return err
		}
		err = path_check("Datastore.Locations",
			config_obj.Datastore.FilestoreDirectory)
		if err != nil {
			return err
		}
	}

	return logging.InitLogging(config_obj)
}

// Loads the client config and merges it with the writeback file.
func LoadConfigWithWriteback(filename string) (*config_proto.Config, error) {
	config_obj, err := LoadConfig(filename)
	if err != nil {
		return nil, err
	}

	existing_writeback := &config_proto.Writeback{}
	data, err := ioutil.ReadFile(WritebackLocation(config_obj))
	// Failing to read the file is not an error - the file may not
	// exist yet.
	if err == nil {
		err = yaml.Unmarshal(data, existing_writeback)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	// Merge the writeback with the config.
	config_obj.Writeback = existing_writeback

	// Validate client config
	return config_obj, ValidateClientConfig(config_obj)
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
