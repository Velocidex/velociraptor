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
package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"strings"

	"github.com/Velocidex/yaml"
	errors "github.com/pkg/errors"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
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

	api_config_path = app.Flag("api_config", "The API configuration file.").Short('a').
			Envar("VELOCIRAPTOR_API_CONFIG").String()

	artifact_definitions_dir = app.Flag(
		"definitions", "A directory containing artifact definitions").String()

	verbose_flag = app.Flag(
		"verbose", "Enabled verbose logging for client.").Short('v').
		Default("false").Bool()

	profile_flag = app.Flag(
		"profile", "Write profiling information to this file.").String()

	trace_flag = app.Flag(
		"trace", "Write trace information to this file.").String()

	command_handlers []CommandHandler
)

func validateServerConfig(configuration *api_proto.Config) error {
	if configuration.Frontend.Certificate == "" {
		return errors.New("Configuration does not specify a frontend certificate.")
	}

	for _, url := range configuration.Client.ServerUrls {
		if !strings.HasSuffix(url, "/") {
			return errors.New(
				"Configuration Client.server_urls must end with /")
		}
	}

	// On windows we require file locations to include a drive
	// letter.
	if runtime.GOOS == "windows" {
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
			configuration.Datastore.Location)
		if err != nil {
			return err
		}
		err = path_check("Datastore.Locations",
			configuration.Datastore.FilestoreDirectory)
		if err != nil {
			return err
		}
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

func maybe_parse_api_config(config_obj *api_proto.Config) {
	if *api_config_path != "" {
		fd, err := os.Open(*api_config_path)
		kingpin.FatalIfError(err, "Unable to read api config.")

		data, err := ioutil.ReadAll(fd)
		kingpin.FatalIfError(err, "Unable to read api config.")
		err = yaml.Unmarshal(data, &config_obj.ApiConfig)
		kingpin.FatalIfError(err, "Unable to decode config.")
	}
}

func get_config_or_default() *api_proto.Config {
	config_obj, err := config.LoadConfig(*config_path)
	if err != nil {
		config_obj = config.GetDefaultConfig()
	}
	maybe_parse_api_config(config_obj)
	return config_obj
}

func main() {
	app.HelpFlag.Short('h')
	app.UsageTemplate(kingpin.CompactUsageTemplate).DefaultEnvars()
	command := kingpin.MustParse(app.Parse(os.Args[1:]))

	// Just display everything in UTC.
	os.Setenv("TZ", "Z")

	if !*verbose_flag {
		logging.SuppressLogging = true
	}

	if *trace_flag != "" {
		f, err := os.Create(*trace_flag)
		kingpin.FatalIfError(err, "trace file.")
		trace.Start(f)
		defer trace.Stop()
	}

	if *profile_flag != "" {
		f2, err := os.Create(*profile_flag)
		kingpin.FatalIfError(err, "Profile file.")

		pprof.StartCPUProfile(f2)
		defer pprof.StopCPUProfile()

	}

	for _, command_handler := range command_handlers {
		if command_handler(command) {
			break
		}
	}
}
