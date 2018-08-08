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

	command_handlers []func(command string) bool
)

func get_config(config_path string) (*config.Config, error) {
	config_obj := config.GetDefaultConfig()
	err := config.LoadConfig(config_path, config_obj)
	return config_obj, err
}

func validateServerConfig(configuration *config.Config) error {
	if configuration.Frontend.Certificate == "" {
		return errors.New("Configuration does not specify a frontend certificate.")
	}

	return nil
}

func get_server_config(config_path string) (*config.Config, error) {
	config_obj := config.GetDefaultConfig()
	err := config.LoadConfig(config_path, config_obj)
	if err == nil {
		err = validateServerConfig(config_obj)
	}

	return config_obj, err
}

func main() {
	command := kingpin.MustParse(app.Parse(os.Args[1:]))

	for _, command_handler := range command_handlers {
		if command_handler(command) {
			break
		}
	}
}
