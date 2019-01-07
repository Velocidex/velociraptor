package main

import (
	"fmt"

	"github.com/Velocidex/yaml"
	"gopkg.in/alecthomas/kingpin.v2"
	config "www.velocidex.com/golang/velociraptor/config"
)

var (
	version = app.Command("version", "Report client version.")
)

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		if command == version.FullCommand() {
			config_obj := config.GetDefaultConfig()
			res, err := yaml.Marshal(config_obj.Version)
			if err != nil {
				kingpin.FatalIfError(err, "Unable to encode version.")
			}

			fmt.Printf("%v", string(res))
			return true
		}
		return false
	})
}
