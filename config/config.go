package config

import (
	"io/ioutil"
	"gopkg.in/yaml.v2"
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
	Client_name string `yaml:"Client.name"`
	Client_private_key string `yaml:"Client.private_key"`
	Client_server_urls StringArray `yaml:"Client.server_urls"`
}


func GetDefaultConfig() Config {
	return Config{
		Client_name: "velociraptor",
	}
}


// Load the config stored in the YAML file and returns a config object.
func LoadConfig(filename string) (*Config, error) {
	result := GetDefaultConfig()

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(data, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}
