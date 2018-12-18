package main

import (
	"os"

	errors "github.com/pkg/errors"
	"gopkg.in/alecthomas/kingpin.v2"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/logging"

	// Import all vql plugins.
	_ "www.velocidex.com/golang/velociraptor/vql_plugins"
)

type CommandHandler func(command string) bool

var (
	app = kingpin.New("velociraptor",
		"An advanced incident response and monitoring agent.")
	config_path = app.Flag("config", "The configuration file.").Short('c').
			Envar("VELOCIRAPTOR_CONFIG").String()

	artifact_definitions_dir = app.Flag(
		"definitions", "A directory containing artifact definitions").String()

	verbose_flag = app.Flag(
		"verbose", "Enabled verbose logging for client.").Short('v').
		Default("false").Bool()

	command_handlers []CommandHandler
)

func validateServerConfig(configuration *api_proto.Config) error {
	if configuration.Frontend.Certificate == "" {
		return errors.New("Configuration does not specify a frontend certificate.")
	}

	return nil
}

func get_server_config(config_path string) (*api_proto.Config, error) {
	config_obj, err := config.LoadConfig(config_path)
	if err != nil {
		return nil, err
	}
	if err == nil {
		err = validateServerConfig(config_obj)
	}

	return config_obj, err
}

func get_config_or_default() *api_proto.Config {
	config_obj, err := config.LoadConfig(*config_path)
	if err != nil {
		config_obj = config.GetDefaultConfig()
	}

	return config_obj
}

func main() {
	app.HelpFlag.Short('h')
	app.UsageTemplate(kingpin.CompactUsageTemplate).DefaultEnvars()
	command := kingpin.MustParse(app.Parse(os.Args[1:]))

	if !*verbose_flag {
		logging.SuppressLogging = true
	}

	for _, command_handler := range command_handlers {
		if command_handler(command) {
			break
		}
	}
}
