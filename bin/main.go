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
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"os"
	"runtime/pprof"
	"runtime/trace"

	"github.com/Velocidex/survey"
	"github.com/Velocidex/yaml/v2"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
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

	run_as = app.Flag("runas", "Run as this username's ACLs").String()

	artifact_definitions_dir = app.Flag(
		"definitions", "A directory containing artifact definitions").String()

	verbose_flag = app.Flag(
		"verbose", "Enabled verbose logging for client.").Short('v').
		Default("false").Bool()

	profile_flag = app.Flag(
		"profile", "Write profiling information to this file.").String()

	trace_flag = app.Flag(
		"trace", "Write trace information to this file.").String()

	trace_vql_flag = app.Flag("trace_vql", "Enable VQL tracing.").Bool()

	command_handlers []CommandHandler
)

func get_server_config(config_path string) (*config_proto.Config, error) {
	config_obj, err := config.LoadConfig(config_path)
	if err != nil {
		return nil, err
	}
	if err == nil {
		err = config.ValidateFrontendConfig(config_obj)
	}

	return config_obj, err
}

func maybe_parse_api_config(config_obj *config_proto.Config) {
	if *api_config_path != "" {
		fd, err := os.Open(*api_config_path)
		kingpin.FatalIfError(err, "Unable to read api config.")

		data, err := ioutil.ReadAll(fd)
		kingpin.FatalIfError(err, "Unable to read api config.")
		err = yaml.Unmarshal(data, &config_obj.ApiConfig)
		kingpin.FatalIfError(err, "Unable to decode config.")

		// If the key is locked ask for a password.
		private_key := []byte(config_obj.ApiConfig.ClientPrivateKey)
		block, _ := pem.Decode(private_key)
		if block == nil {
			kingpin.Fatalf("Unable to decode private key.")
		}

		if x509.IsEncryptedPEMBlock(block) {
			password := ""
			err := survey.AskOne(
				&survey.Password{Message: "Password:"},
				&password,
				survey.WithValidator(survey.Required))
			kingpin.FatalIfError(err, "Password.")

			decrypted_block, err := x509.DecryptPEMBlock(
				block, []byte(password))
			kingpin.FatalIfError(err, "Password.")

			config_obj.ApiConfig.ClientPrivateKey = string(
				pem.EncodeToMemory(&pem.Block{
					Bytes: decrypted_block,
					Type:  block.Type,
				}))
		}

	}
}

func load_config_or_api() (*config_proto.Config, error) {
	config_obj, err := config.LoadConfig(*config_path)
	if err != nil {
		return nil, err
	}
	if config_obj.Frontend != nil {
		err = config.ValidateFrontendConfig(config_obj)
	} else {
		err = config.ValidateClientConfig(config_obj)
	}

	if err != nil {
		return nil, err
	}

	maybe_parse_api_config(config_obj)
	load_config_artifacts(config_obj)

	return config_obj, nil
}

func load_config_or_default() *config_proto.Config {
	config_obj, err := load_config_or_api()
	if err != nil {
		return &config_proto.Config{}
	}

	// Initialize the logging now that we have loaded the config.
	err = logging.InitLogging(config_obj)
	kingpin.FatalIfError(err, "Logging")

	return config_obj
}

func main() {
	app.HelpFlag.Short('h')
	app.UsageTemplate(kingpin.CompactUsageTemplate).DefaultEnvars()
	args := os.Args[1:]

	var embedded_config *config_proto.Config

	// If no args are given check if there is an embedded config
	// with autoexec.
	if len(args) == 0 {
		args, embedded_config = maybeUnpackConfig(args)
	}

	command := kingpin.MustParse(app.Parse(args))

	if !*verbose_flag {
		logging.SuppressLogging = true
		logging.Manager.Reset()

		// We need to delay this message until we parsed the
		// command line so we can find out of the logging is
		// suppressed.
		if embedded_config != nil && embedded_config.Autoexec != nil {
			logging.GetLogger(embedded_config, &logging.ToolComponent).
				Info("Autoexec with parameters: %s",
					embedded_config.Autoexec)
		}
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
