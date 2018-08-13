package main

import (
	errors "github.com/pkg/errors"
	"gopkg.in/alecthomas/kingpin.v2"
	"os"
	"www.velocidex.com/golang/velociraptor/config"
)

var (
	app         = kingpin.New("velociraptor", "An advanced incident response agent.")
	config_path = app.Flag("config", "The configuration file.").String()

	artifact_definitions_dir = app.Flag(
		"definitions", "A directory containing artifact definitions").String()

	command_handlers []func(command string) bool
)

func validateServerConfig(configuration *config.Config) error {
	if configuration.Frontend.Certificate == "" {
		return errors.New("Configuration does not specify a frontend certificate.")
	}

	return nil
}

func get_server_config(config_path string) (*config.Config, error) {
	config_obj, err := config.LoadConfig(config_path)
	if err != nil {
		return nil, err
	}
	if err == nil {
		err = validateServerConfig(config_obj)
	}

	return config_obj, err
}

func get_config_or_default() *config.Config {
	config_obj, err := config.LoadConfig(*config_path)
	if err != nil {
		config_obj = config.GetDefaultConfig()
	}

	return config_obj
}

func main() {
	command := kingpin.MustParse(app.Parse(os.Args[1:]))

	for _, command_handler := range command_handlers {
		if command_handler(command) {
			break
		}
	}
}
