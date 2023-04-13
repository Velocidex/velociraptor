/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2022 Rapid7 Inc.

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

	"github.com/AlecAivazis/survey/v2"
	errors "github.com/go-errors/errors"
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

	override_flag = app.Flag("config_override", "A json object to override the config.").
			Short('o').String()

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

	tempdir_flag = app.Flag(
		"tempdir", "Write all temp files to this directory").String()

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
	APIConfigLoader *config.Loader
	default_config  *config_proto.Config
)

func main() {
	app.HelpFlag.Short('h')
	app.UsageTemplate(kingpin.CompactUsageTemplate)
	args := os.Args[1:]

	// If no args are given check if there is an embedded config
	// with autoexec.
	pre, post := splitArgs(args)
	if len(pre) == 0 {
		config_obj, err := new(config.Loader).WithVerbose(*verbose_flag).
			WithEmbedded().LoadAndValidate()
		if err == nil && config_obj.Autoexec != nil && config_obj.Autoexec.Argv != nil {
			args = nil
			for _, arg := range config_obj.Autoexec.Argv {
				args = append(args, os.ExpandEnv(arg))
			}
			args = append(args, post...)
			logging.Prelog("Autoexec with parameters: %v", args)
		}
	}

	// Automatically add config flags
	var err error
	default_config, err = parseFlagsToDefaultConfig(app)
	kingpin.FatalIfError(err, "Adding config flags.")

	command := kingpin.MustParse(app.Parse(args))

	if *no_color_flag {
		logging.NoColor = true
	}

	doBanner()
	defer doPrompt()

	// Commands that potentially take an API config can load both
	// - first try the API config, then try a config.
	APIConfigLoader = new(config.Loader).WithVerbose(*verbose_flag).
		WithTempdir(*tempdir_flag).
		WithApiLoader(*api_config_path).
		WithEnvApiLoader("VELOCIRAPTOR_API_CONFIG").
		WithCustomValidator("Validator maybe_unlock_api_config",
			maybe_unlock_api_config).
		WithFileLoader(*config_path).
		WithEmbedded().
		WithEnvLoader("VELOCIRAPTOR_CONFIG").
		WithConfigMutator("Mutator mergeFlagConfig",
			func(config_obj *config_proto.Config) error {
				return mergeFlagConfig(config_obj, default_config)
			}).
		WithCustomValidator("validator: initFilestoreAccessor",
			initFilestoreAccessor).
		WithCustomValidator("validator: initDebugServer", initDebugServer).
		WithCustomValidator("validator: timezone", initTimezone).
		WithConfigMutator("Mutator: applyMinionRole", applyMinionRole).
		WithCustomValidator("validator: applyAnalysisTarget",
			applyAnalysisTarget).
		WithOverride(*override_flag).
		WithLogFile(*logging_flag).
		WithConfigMutator("Mutator maybeAddDefinitionsDirectory", maybeAddDefinitionsDirectory)

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

func makeDefaultConfigLoader() *config.Loader {
	return new(config.Loader).
		WithVerbose(*verbose_flag).
		WithTempdir(*tempdir_flag).
		WithFileLoader(*config_path).
		WithEmbedded().
		WithEnvLoader("VELOCIRAPTOR_CONFIG").
		WithConfigMutator("Mutator mergeFlagConfig",
			func(config_obj *config_proto.Config) error {
				return mergeFlagConfig(config_obj, default_config)
			}).
		WithCustomValidator("validator: initFilestoreAccessor",
			initFilestoreAccessor).
		WithCustomValidator("validator: initDebugServer", initDebugServer).
		WithCustomValidator("validator: timezone", initTimezone).
		WithLogFile(*logging_flag).
		WithOverride(*override_flag).
		WithConfigMutator("Mutator applyMinionRole", applyMinionRole).
		WithCustomValidator("validator: ensureProxy", ensureProxy).
		WithConfigMutator("Mutator applyAnalysisTarget", applyAnalysisTarget).
		WithConfigMutator("Mutator maybeAddDefinitionsDirectory", maybeAddDefinitionsDirectory)
}

// Split the command line into args before the -- and after the --
func splitArgs(args []string) (pre, post []string) {
	seen := false
	for _, arg := range args {
		if arg == "--" {
			seen = true
			continue
		}

		if seen {
			post = append(post, arg)
		} else {
			pre = append(pre, arg)
		}
	}

	return pre, post
}
