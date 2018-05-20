package config

import (
	"github.com/golang/protobuf/proto"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
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

type Config struct {
	Client_name        *string     `yaml:"Client.name,omitempty"`
	Client_description *string     `yaml:"Client.description,omitempty"`
	Client_version     *uint32     `yaml:"Client.version,omitempty"`
	Client_commit      *string     `yaml:"Client.commit,omitempty"`
	Client_build_time  *string     `yaml:"Client.build_time,omitempty"`
	Client_labels      StringArray `yaml:"Client.labels,omitempty"`

	Client_private_key *string     `yaml:"Client.private_key,omitempty"`
	Client_server_urls StringArray `yaml:"Client.server_urls,omitempty"`

	// We store local configuration on this file.
	Config_writeback *string `yaml:"Config.writeback,omitempty"`
}

func GetDefaultConfig() *Config {
	return &Config{
		Client_name:       proto.String("velociraptor"),
		Client_version:    proto.Uint32(1),
		Client_build_time: &build_time,
		Client_commit:     &commit_hash,
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
	err = ioutil.WriteFile(filename, bytes, os.ModePerm)
	if err != nil {
		return err
	}

	return nil
}
