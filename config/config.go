package config

import (
	"github.com/ghodss/yaml"
	errors "github.com/pkg/errors"
	"io/ioutil"
	"runtime"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
)

// Embed build time constants into here for reporting client version.
// https://husobee.github.io/golang/compile/time/variables/2015/12/03/compile-time-const.html
var (
	build_time  string
	commit_hash string
)

type Config struct {
	*api_proto.Config

	// A virtual field which is calculated from
	// Client.WritebackLinux, Client.WritebackWindows etc.
	Writeback string `json:"-"`
}

// Get an empty client config.
func NewClientConfig() *Config {
	return &Config{
		&api_proto.Config{
			Client: &api_proto.ClientConfig{},
		}, ""}
}

// Create a default configuration object.
func GetDefaultConfig() *Config {
	return &Config{
		&api_proto.Config{
			Client: &api_proto.ClientConfig{
				Name:           "velociraptor",
				Version:        "0.1",
				BuildTime:      build_time,
				Commit:         commit_hash,
				WritebackLinux: "/etc/velociraptor.writeback.yaml",
				WritebackWindows: "/Program Files/Velociraptor/" +
					"velociraptor.writeback.yaml",
			},
			API: &api_proto.APIConfig{
				// Bind port for gRPC endpoint - this should not
				// normally be exposed.
				BindAddress: "127.0.0.1",
				BindPort:    8888,
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
				BindAddress:     "127.0.0.1",
				BindPort:        8000,
				ClientLeaseTime: 600,
			},
			Datastore: &api_proto.DatastoreConfig{
				Implementation:     "FileBaseDataStore",
				Location:           "/tmp/velociraptor",
				FilestoreDirectory: "/tmp/velociraptor",
			},
			Flows: &api_proto.FlowsConfig{},
		}, "",
	}
}

// Load the config stored in the YAML file and returns a config object.
func LoadConfig(filename string, config *Config) error {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return errors.WithStack(err)
	}

	err = yaml.Unmarshal(data, config)
	if err != nil {
		return errors.WithStack(err)
	}

	switch runtime.GOOS {
	case "linux":
		config.Writeback = config.Client.WritebackLinux
	case "windows":
		config.Writeback = config.Client.WritebackWindows
	default:
		config.Writeback = config.Client.WritebackLinux
	}

	return nil
}

func ParseConfigFromString(config_string []byte, config *Config) error {
	err := yaml.Unmarshal(config_string, config)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func Encode(config *Config) ([]byte, error) {
	res, err := yaml.Marshal(config)
	return res, err
}

func WriteConfigToFile(filename string, config *Config) error {
	bytes, err := Encode(config)
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
