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
	"os"
	"runtime/pprof"
	"runtime/trace"

	"github.com/Velocidex/survey"
	errors "github.com/pkg/errors"
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

	config_path = app.Flag("config", "The configuration file.").
			Short('c').String()
	api_config_path = app.Flag("api_config", "The API configuration file.").
			Short('a').String()

	run_as = app.Flag("runas", "Run as this username's ACLs").String()

	artifact_definitions_dir = app.Flag(
		"definitions", "A directory containing artifact definitions").String()

	no_color_flag = app.Flag("nocolor", "Disable color output").Bool()

	verbose_flag = app.Flag(
		"verbose", "Enabled verbose logging.").Short('v').
		Default("false").Bool()

	profile_flag = app.Flag(
		"profile", "Write profiling information to this file.").String()

	trace_flag = app.Flag(
		"trace", "Write trace information to this file.").String()

	trace_vql_flag = app.Flag("trace_vql", "Enable VQL tracing.").Bool()

	logging_flag = app.Flag(
		"logfile", "Write to this file as well").String()

	command_handlers []CommandHandler
)

// Try to unlock encrypted API keys
func maybe_unlock_api_config(config_obj *config_proto.Config) error {
	if config_obj.ApiConfig == nil || config_obj.ApiConfig.ClientPrivateKey == "" {
		return nil
	}

	// If the key is locked ask for a password.
	private_key := []byte(config_obj.ApiConfig.ClientPrivateKey)
	block, _ := pem.Decode(private_key)
	if block == nil {
		return errors.New("Unable to decode private key.")
	}

	if x509.IsEncryptedPEMBlock(block) {
		password := ""
		err := survey.AskOne(
			&survey.Password{Message: "Password:"},
			&password,
			survey.WithValidator(survey.Required))
		if err != nil {
			return err
		}

		decrypted_block, err := x509.DecryptPEMBlock(
			block, []byte(password))
		if err != nil {
			return err
		}
		config_obj.ApiConfig.ClientPrivateKey = string(
			pem.EncodeToMemory(&pem.Block{
				Bytes: decrypted_block,
				Type:  block.Type,
			}))
	}
	return nil
}

var (
	DefaultConfigLoader, APIConfigLoader *config.Loader
)

func main() {
	app.HelpFlag.Short('h')
	app.UsageTemplate(kingpin.CompactUsageTemplate)
	args := os.Args[1:]

	// If no args are given check if there is an embedded config
	// with autoexec.
	if len(args) == 0 {
		config_obj, err := new(config.Loader).WithVerbose(*verbose_flag).
			WithEmbedded().LoadAndValidate()
		if err == nil && config_obj.Autoexec != nil && config_obj.Autoexec.Argv != nil {
			for _, arg := range config_obj.Autoexec.Argv {
				args = append(args, os.ExpandEnv(arg))
			}
			logging.Prelog("Autoexec with parameters: %v", args)
		}
	}

	command := kingpin.MustParse(app.Parse(args))

	if *no_color_flag {
		logging.NoColor = true
	}

	doBanner()
	defer doPrompt()

	// Most commands load a config in the following order
	DefaultConfigLoader = new(config.Loader).WithVerbose(*verbose_flag).
		WithFileLoader(*config_path).
		WithEmbedded().
		WithEnvLoader("VELOCIRAPTOR_CONFIG").
		WithCustomValidator(initFilestoreAccessor).
		WithCustomValidator(initDebugServer).
		WithLogFile(*logging_flag)

	// Commands that potentially take an API config can load both
	// - first try the API config, then try a config.
	APIConfigLoader = new(config.Loader).WithVerbose(*verbose_flag).
		WithApiLoader(*api_config_path).
		WithEnvApiLoader("VELOCIRAPTOR_API_CONFIG").
		WithCustomValidator(maybe_unlock_api_config).
		WithFileLoader(*config_path).
		WithEmbedded().
		WithEnvLoader("VELOCIRAPTOR_CONFIG").
		WithCustomValidator(initFilestoreAccessor).
		WithCustomValidator(initDebugServer).
		WithLogFile(*logging_flag)

	if *trace_flag != "" {
		f, err := os.Create(*trace_flag)
		kingpin.FatalIfError(err, "trace file.")
		err = trace.Start(f)
		kingpin.FatalIfError(err, "trace file.")
		defer trace.Stop()
	}

	if *profile_flag != "" {
		f2, err := os.Create(*profile_flag)
		kingpin.FatalIfError(err, "Profile file.")

		err = pprof.StartCPUProfile(f2)
		kingpin.FatalIfError(err, "Profile file.")
		defer pprof.StopCPUProfile()

	}

	for _, command_handler := range command_handlers {
		if command_handler(command) {
			break
		}
	}
}
