package config

import (
	"github.com/golang/protobuf/proto"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"strconv"
)

// Embed build time constants into here for reporting client version.
// https://husobee.github.io/golang/compile/time/variables/2015/12/03/compile-time-const.html
var (
	build_time  string
	commit_hash string
)

// Can be an array or a string in YAML but always parses to a string
// array. GRR Config files are inconsistent in this regard so to
// interoperate we need to support this
// ambiguity. (https://github.com/go-yaml/yaml/issues/100)
type StringArray []string

func (a *StringArray) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var multi []string
	err := unmarshal(&multi)
	if err != nil {
		var single string
		err := unmarshal(&single)
		if err != nil {
			return err
		}
		*a = []string{single}
	} else {
		*a = multi
	}
	return nil
}

// GRR erroneously writes all YAML fields as strings even Integers
//  (e.g. Frontend_bind_port: '8080' instead of Frontend_bind_port:
//  8080), so we need to be able to handle either form.
type Integer uint64

func (self *Integer) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var maybe_string string
	var maybe_int uint64

	err := unmarshal(&maybe_string)
	if err == nil {
		maybe_int, err = strconv.ParseUint(maybe_string, 10, 64)
		if err != nil {
			return err
		}
		*self = Integer(maybe_int)
		return nil
	}

	err = unmarshal(&maybe_int)
	if err != nil {
		return err
	}

	*self = Integer(maybe_int)
	return nil
}

type Config struct {
	Client_name        *string     `yaml:"Client.name,omitempty"`
	Client_description *string     `yaml:"Client.description,omitempty"`
	Client_version     *uint32     `yaml:"Client.version,omitempty"`
	Client_commit      *string     `yaml:"Client.commit,omitempty"`
	Client_build_time  *string     `yaml:"Client.build_time,omitempty"`
	Client_labels      StringArray `yaml:"Client.labels,omitempty"`

	Client_private_key *string     `yaml:"Client.private_key,omitempty"`
	Client_server_urls StringArray `yaml:"Client.server_urls,omitempty"`

	// We store local configuration in this file.
	Config_writeback *string `yaml:"Config.writeback,omitempty"`

	// GRPC API endpoint.
	API_bind_address       *string `yaml:"API.bind_address,omitempty"`
	API_bind_port          *uint32 `yaml:"API.bind_port,omitempty"`
	API_proxy_bind_address *string `yaml:"API.proxy_bind_address,omitempty"`
	API_proxy_bind_port    *uint32 `yaml:"API.proxy_bind_port,omitempty"`

	Frontend_bind_address *string  `yaml:"Frontend.bind_address,omitempty"`
	Frontend_bind_port    *Integer `yaml:"Frontend.bind_port,omitempty"`
	Frontend_certificate  *string  `yaml:"Frontend.certificate,omitempty"`

	Frontend_private_key *string `yaml:"PrivateKeys.server_key,omitempty"`

	// DataStore parameters.
	Datastore_implementation *string `yaml:"Datastore.implementation,omitempty"`
	Datastore_location       *string `yaml:"Datastore.location,omitempty"`

	// The Admin UI
	AdminUI_document_root *string `yaml:"AdminUI.document_root,omitempty"`

	// File Store
	FileStore_directory *string `yaml:"FileStore.directory,omitempty"`
}

func GetDefaultConfig() *Config {
	bind_port := Integer(8080)
	return &Config{
		Client_name:       proto.String("velociraptor"),
		Client_version:    proto.Uint32(1),
		Client_build_time: &build_time,
		Client_commit:     &commit_hash,

		Frontend_bind_address: proto.String(""),
		Frontend_bind_port:    &bind_port,

		API_bind_address:       proto.String("localhost"),
		API_bind_port:          proto.Uint32(8888),
		API_proxy_bind_address: proto.String("localhost"),
		API_proxy_bind_port:    proto.Uint32(8889),
	}
}

// Load the config stored in the YAML file and returns a config object.
func LoadConfig(filename string, config *Config) error {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(data, config)
	if err != nil {
		return err
	}

	return nil
}

func ParseConfigFromString(config_string []byte, config *Config) error {
	err := yaml.Unmarshal(config_string, config)
	if err != nil {
		return err
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
		return err
	}

	return nil
}
