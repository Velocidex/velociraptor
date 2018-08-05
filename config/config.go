package config

import (
	"github.com/ghodss/yaml"
	"github.com/golang/protobuf/proto"
	errors "github.com/pkg/errors"
	"io/ioutil"
	"strconv"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
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
			return errors.WithStack(err)
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
			return errors.WithStack(err)
		}
		*self = Integer(maybe_int)
		return nil
	}

	err = unmarshal(&maybe_int)
	if err != nil {
		return errors.WithStack(err)
	}

	*self = Integer(maybe_int)
	return nil
}

type Config struct {
	Client_name        *string     `json:"Client.name,omitempty"`
	Client_description *string     `json:"Client.description,omitempty"`
	Client_version     *uint32     `json:"Client.version,omitempty"`
	Client_commit      *string     `json:"Client.commit,omitempty"`
	Client_build_time  *string     `json:"Client.build_time,omitempty"`
	Client_labels      StringArray `json:"Client.labels,omitempty"`

	Client_private_key *string     `json:"Client.private_key,omitempty"`
	Client_server_urls StringArray `json:"Client.server_urls,omitempty"`

	// We store local configuration in this file.
	Config_writeback *string `json:"Config.writeback,omitempty"`

	// GRPC API endpoint.
	API_bind_address *string `json:"API.bind_address,omitempty"`
	API_bind_port    *uint32 `json:"API.bind_port,omitempty"`
	GUI_bind_address *string `json:"GUI.bind_address,omitempty"`
	GUI_bind_port    *uint32 `json:"GUI.bind_port,omitempty"`

	// CA
	CA_certificate *string `json:"CA.certificate,omitempty"`
	CA_private_key *string `json:"CA.private_key,omitempty"`

	Frontend_bind_address  *string     `json:"Frontend.bind_address,omitempty"`
	Frontend_bind_port     *Integer    `json:"Frontend.bind_port,omitempty"`
	Frontend_certificate   *string     `json:"Frontend.certificate,omitempty"`
	Frontend_private_key   *string     `json:"Frontend.private_key,omitempty"`
	Frontend_internal_cidr StringArray `json:"Frontend.internal_cidr"`
	Frontend_vpn_cidr      StringArray `json:"Frontend.vpn_cidr"`

	// Time to lease client requests before they are retransmitted (in seconds).
	Frontend_client_lease_time *uint32 `json:"Frontend.client_lease_time,omitempty"`

	// DataStore parameters.
	Datastore_implementation *string `json:"Datastore.implementation,omitempty"`
	Datastore_location       *string `json:"Datastore.location,omitempty"`

	// File Store
	FileStore_directory *string `json:"FileStore.directory,omitempty"`

	// Hunts
	Hunts_last_timestamp *uint64 `json:"Hunts.last_timestamp,omitempty"`

	Interrogate_additional_queries *actions_proto.VQLCollectorArgs `json:"Interrogate.additional_queries,omitempty"`
}

func GetDefaultConfig() *Config {
	bind_port := Integer(8080)
	return &Config{
		Client_name:       proto.String("velociraptor"),
		Client_version:    proto.Uint32(1),
		Client_build_time: &build_time,
		Client_commit:     &commit_hash,

		Frontend_bind_address:      proto.String(""),
		Frontend_bind_port:         &bind_port,
		Frontend_client_lease_time: proto.Uint32(600),
		Frontend_internal_cidr: []string{
			"127.0.0.1/12", "192.168.0.0/16",
		},

		API_bind_address: proto.String("localhost"),
		API_bind_port:    proto.Uint32(8888),
		GUI_bind_address: proto.String("localhost"),
		GUI_bind_port:    proto.Uint32(8889),

		Hunts_last_timestamp: proto.Uint64(0),
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
