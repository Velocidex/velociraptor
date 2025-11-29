/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

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
	"fmt"
	"os"
	"runtime/pprof"
	"runtime/trace"
	"time"

	kingpin "github.com/alecthomas/kingpin/v2"
	errors "github.com/go-errors/errors"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/logging"
	vsurvey "www.velocidex.com/golang/velociraptor/tools/survey"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/proxy"

	// Import all vql plugins.
	_ "www.velocidex.com/golang/velociraptor/vql_plugins"
)

type CommandHandler func(command string) bool

var (
	app = kingpin.New("velociraptor",
		"An advanced incident response and monitoring agent.")

	config_path = app.Flag("config", "The configuration file.").
			Short('c').String()

	embedded_config_path = app.Flag("embedded_config", "Extract the embedded configuration from this file.").String()

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

	profile_duration = app.Flag(
		"profile_duration", "Generate a profile file for each period in seconds.").
		Int64()

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
		password, err := vsurvey.GetAPIClientDecryptPassword()
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
	app.Terminate(func(s int) {
		doPrompt()
		os.Exit(s)
	})

	args, err := transformArgv(os.Args[1:])
	kingpin.FatalIfError(err, "Command line.")

	// If no args are given check if there is an embedded config
	// with autoexec.
	pre, post := splitArgs(args)
	if len(pre) == 0 {
		config_obj, err := new(config.Loader).WithVerbose(*verbose_flag).
			WithEmbedded(*embedded_config_path).LoadAndValidate()
		if err == nil && config_obj.Autoexec != nil && config_obj.Autoexec.Argv != nil {
			args = nil
			for _, arg := range config_obj.Autoexec.Argv {
				args = append(args, utils.ExpandEnv(arg))
			}
			args = append(args, post...)
			Prelog("Autoexec with parameters: %v", args)
		}
	}

	// Log the actual argv that will be run.
	err = logArgv(append([]string{os.Args[0]}, args...))
	if err != nil {
		fmt.Printf("Error sending to the event log: %v\n", err)
	}

	// Automatically add config flags
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
		WithEnvApiLoader(constants.VELOCIRAPTOR_API_CONFIG).
		WithCustomValidator("Validator maybe_unlock_api_config",
			maybe_unlock_api_config).
		WithFileLoader(*config_path).
		WithEmbedded(*embedded_config_path).
		WithEnvLoader(constants.VELOCIRAPTOR_CONFIG).
		WithEnvLiteralLoader(constants.VELOCIRAPTOR_LITERAL_CONFIG).
		WithConfigMutator("Mutator mergeFlagConfig",
			func(config_obj *config_proto.Config) error {
				return mergeFlagConfig(config_obj, default_config)
			}).
		WithCustomValidator("validator: initFilestoreAccessor",
			initFilestoreAccessor).
		WithCustomValidator("validator: initDebugServer", initDebugServer).
		WithCustomValidator("validator: timezone", initTimezone).
		WithConfigMutator("Mutator: applyMinionRole", applyMinionRole).
		WithCustomValidator("validator: ensureProxy", proxy.ConfigureProxy).
		WithCustomValidator("validator: applyRemapping", applyRemapping).
		WithConfigMutator("OverrideFlag", deprecatedOverride).
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
		if *profile_duration > 0 {
			go func() {
				for i := 0; ; i++ {
					filename := fmt.Sprintf("%s_%d.profile", *profile_flag, i)
					fmt.Printf("Writing profile at %v", filename)
					f2, err := os.Create(filename)
					if err != nil {
						return
					}

					err = pprof.StartCPUProfile(f2)
					if err == nil {
						time.Sleep(time.Duration(*profile_duration) * time.Second)
						pprof.StopCPUProfile()
					}
					f2.Close()
				}
			}()

		} else {
			f2, err := os.Create(*profile_flag)
			kingpin.FatalIfError(err, "Profile file.")

			err = pprof.StartCPUProfile(f2)
			kingpin.FatalIfError(err, "Profile file.")
			defer pprof.StopCPUProfile()
		}
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
		WithEmbedded(*embedded_config_path).
		WithEnvLoader(constants.VELOCIRAPTOR_CONFIG).
		WithEnvLiteralLoader(constants.VELOCIRAPTOR_LITERAL_CONFIG).
		WithConfigMutator("Mutator mergeFlagConfig",
			func(config_obj *config_proto.Config) error {
				return mergeFlagConfig(config_obj, default_config)
			}).
		WithCustomValidator("validator: initFilestoreAccessor",
			initFilestoreAccessor).
		WithCustomValidator("validator: initDebugServer", initDebugServer).
		WithCustomValidator("validator: timezone", initTimezone).
		WithLogFile(*logging_flag).
		WithConfigMutator("OverrideFlag", deprecatedOverride).
		WithConfigMutator("Mutator applyMinionRole", applyMinionRole).
		WithCustomValidator("validator: ensureProxy", proxy.ConfigureProxy).
		WithConfigMutator("Mutator applyRemapping", applyRemapping).
		WithConfigMutator("Mutator maybeAddDefinitionsDirectory", maybeAddDefinitionsDirectory)
}

// Split the command line into args before the -- and after the --
func splitArgs(args []string) (pre, post []string) {
	seen := false
	for idx := 0; idx < len(args); idx++ {
		arg := args[idx]

		// Separate the args into pre and post args. Post args will be
		// added to the autoexec command line while still triggering
		// the autoexec condition.
		if arg == "--" {
			seen = true
			continue
		}

		if arg == "--embedded_config" && idx < len(args) {
			embedded_config_path = &args[idx+1]
		}

		if seen {
			post = append(post, arg)
		} else {
			pre = append(pre, arg)
		}
	}

	return pre, post
}
