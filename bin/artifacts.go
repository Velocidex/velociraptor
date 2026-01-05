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
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/yaml/v2"
	errors "github.com/go-errors/errors"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/executor"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/startup"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
)

var (
	artifact_command = app.Command(
		"artifacts", "Process artifact definitions.")

	artifact_command_list = artifact_command.Command(
		"list", "Print all artifacts")

	artifact_command_show = artifact_command.Command(
		"show", "Show an artifact")

	artifact_command_collect_cli_mode = artifact_command_collect.Flag(
		"cli_help_mode", "Display help like a CLI command").
		Bool()

	artifact_command_show_name = artifact_command_show.Arg(
		"name", "Name to show.").Required().String()

	artifact_command_list_name = artifact_command_list.Arg(
		"regex", "Regex of names to match.").
		HintAction(listArtifactsHint).String()

	artifact_command_list_verbose_count = artifact_command_list.Flag(
		"details", "Show more details (Use -d -dd for even more)").
		Short('d').Counter()

	artifact_command_collect = artifact_command.Command(
		"collect", "Collect all artifacts")

	artifact_command_collect_output = artifact_command_collect.Flag(
		"output", "When specified we create a zip file and "+
			"store all output in it.").
		Default("").String()

	artifact_command_collect_timeout = artifact_command_collect.Flag(
		"timeout", "Time collection out after this many seconds.").
		Default("0").Float64()

	artifact_command_collect_progress_timeout = artifact_command_collect.Flag(
		"progress_timeout", "If specified we terminate the colleciton if no progress is made in this many seconds.").
		Default("0").Float64()

	artifact_command_collect_cpu_limit = artifact_command_collect.Flag(
		"cpu_limit", "A number between 0 to 100 representing maximum CPU utilization.").
		Default("0").Int64()

	artifact_command_collect_output_compression = artifact_command_collect.Flag(
		"output_level", "Compression level for zip output.").
		Default("5").Int64()

	artificat_command_collect_admin_flag = artifact_command_collect.Flag(
		"require_admin", "Ensure the user is an admin").Bool()

	artifact_command_collect_output_password = artifact_command_collect.Flag(
		"password", "When specified we encrypt zip file with this password.").
		Default("").String()

	artifact_command_collect_format = artifact_command_collect.Flag(
		"format", "Output format to use  (csv, json, csv_only).").
		Default("json").Enum("json", "jsonl", "csv", "csv_only")

	artifact_command_collect_names = artifact_command_collect.Arg(
		"artifact_name", "The artifact name to collect.").
		Required().HintAction(listArtifactsHint).Strings()

	artifact_command_collect_args = artifact_command_collect.Flag(
		"args", "Artifact args (e.g. --args Foo=Bar).").Strings()

	artifact_command_collect_hardmemory = artifact_command_collect.Flag(
		"hard_memory_limit", "If we reach this memory limit in bytes we exit.").Uint64()
)

func listArtifactsHint() []string {
	config_obj := config.GetDefaultConfig()
	ctx := context.Background()
	result := []string{}

	repository, err := getRepository(config_obj)
	if err != nil {
		return result
	}
	names, err := repository.List(ctx, config_obj)
	if err != nil {
		return result
	}

	result = append(result, names...)
	return result
}

func getRepository(config_obj *config_proto.Config) (services.Repository, error) {
	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return nil, err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}

	return repository, nil
}

func doArtifactCollect() error {
	logging.DisableLogging()

	if *artificat_command_collect_admin_flag {
		err := checkAdmin()
		if err != nil {
			return err
		}
	}

	config_obj, err := makeDefaultConfigLoader().
		WithNullLoader().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to create config: %w", err)
	}

	ctx, top_cancel := install_sig_handler()
	defer top_cancel()

	sm, err := startup.StartToolServices(ctx, config_obj)
	defer sm.Close()

	if err != nil {
		return err
	}

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	spec := ordereddict.NewDict()
	for _, name := range *artifact_command_collect_names {
		collect_args := ordereddict.NewDict()
		for _, item := range *artifact_command_collect_args {
			// If an arg has to apply only to a single
			// artifact then the user may prepend the name
			// of the artifact with a :: separator.
			namespaces := strings.SplitN(item, "::", 2)
			if len(namespaces) == 2 {
				if namespaces[0] != name {
					continue
				}
				item = namespaces[1]
			}

			parts := strings.SplitN(item, "=", 2)
			arg_name := parts[0]

			arg_type, err := getParameterType(ctx, config_obj,
				repository, name, arg_name)
			if err != nil {
				return err
			}

			if len(parts) < 2 {
				collect_args.Set(arg_name, "Y")
			} else {
				collect_args.Set(arg_name,
					parseArtifactType(arg_type, parts[1]))
			}
		}

		spec.Set(name, collect_args)
	}

	if len(*artifact_command_collect_names) == 0 {
		return errors.New("Need some artifact to collect")
	}

	if *artifact_command_collect_cli_mode {
		for _, name := range *artifact_command_collect_names {
			return doArtifactCLIHelp(ctx, config_obj, manager, name)
		}
	}

	logger := &LogWriter{config_obj: config_obj}
	scope := manager.BuildScope(services.ScopeBuilder{
		Config:     config_obj,
		ACLManager: acl_managers.NullACLManager{},
		Logger:     log.New(logger, "", 0),
		Env: ordereddict.NewDict().
			Set("Artifacts", *artifact_command_collect_names).
			Set("Output", *artifact_command_collect_output).
			Set("Level", *artifact_command_collect_output_compression).
			Set("Password", *artifact_command_collect_output_password).
			Set("Args", spec).
			Set("Format", *artifact_command_collect_format).
			Set("Timeout", *artifact_command_collect_timeout).
			Set("ProgressTimeout", *artifact_command_collect_progress_timeout).
			Set("CpuLimit", *artifact_command_collect_cpu_limit),
	})
	defer scope.Close()

	// Stick around until the query completes so it gets a chance to
	// close the collection zip.
	sm.Wg.Add(1)
	err = scope.AddDestructor(func() {
		sm.Wg.Done()
	})
	if err != nil {
		sm.Wg.Done()
		return err
	}

	// If interrupt has occured we cancel everything and wait for any
	// cleanups to occur. If we return too quickly from the main
	// thread, we might leave some tempfiles behind.
	defer func() {
		select {
		case <-ctx.Done():
			scope.Log("ERROR:Interrupted! Sleeping on exit to allow cleanup")
			time.Sleep(time.Second)
		default:
		}
	}()

	if *artifact_command_collect_hardmemory > 0 {
		scope.Log("Installing hard memory limit of %v bytes",
			*artifact_command_collect_hardmemory)
		Nanny := &executor.NannyService{
			MaxMemoryHardLimit: *artifact_command_collect_hardmemory,
			Logger: logging.GetLogger(
				config_obj, &logging.ToolComponent),
		}
		Nanny.RegisterOnWarnings(utils.GetId(), sm.Close)

		// Keep the nanny running after the query is done so it can
		// hard kill the process if cancellation is not enough.
		Nanny.Start(ctx, &sync.WaitGroup{})
	}

	start := utils.GetTime().Now()
	defer func() {
		logging.GetLogger(config_obj, &logging.ToolComponent).
			Info("Collection completed in %v Seconds",
				utils.GetTime().Now().Sub(start))
	}()

	if *trace_vql_flag {
		scope.SetTracer(logging.NewPlainLogger(config_obj,
			&logging.ToolComponent))
	}

	query := `
  SELECT * FROM collect(
     artifacts=Artifacts, output=Output,
     level=Level, timeout=Timeout, progress_timeout=ProgressTimeout,
     cpu_limit=CpuLimit, password=Password, args=Args, format=Format)`
	err = eval_local_query(
		sm.Ctx, config_obj,
		*artifact_command_collect_format, query, scope)
	if err != nil {
		return err
	}

	return logger.Error
}

func getFilterRegEx(pattern string) (*regexp.Regexp, error) {
	pattern = strings.Replace(pattern, "*", ".*", -1)
	pattern = "^" + pattern + "$"
	return regexp.Compile(pattern)
}

func doArtifactShow() error {
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().
		WithNullLoader().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to create config: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	sm, err := startup.StartToolServices(ctx, config_obj)
	defer sm.Close()

	if err != nil {
		return err
	}

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	artifact, pres := repository.Get(ctx, config_obj,
		*artifact_command_show_name)
	if !pres {
		return fmt.Errorf("Artifact %s not found",
			*artifact_command_show_name)
	}

	fmt.Println(artifact.Raw)
	return nil
}

func doArtifactList() error {
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().
		WithNullLoader().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	sm, err := startup.StartToolServices(ctx, config_obj)
	defer sm.Close()

	if err != nil {
		return err
	}

	var name_regex *regexp.Regexp
	if *artifact_command_list_name != "" {
		re, err := getFilterRegEx(*artifact_command_list_name)
		if err != nil {
			return fmt.Errorf("Artifact name regex not valid: %w", err)
		}

		name_regex = re
	}

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	names, err := repository.List(sm.Ctx, config_obj)
	if err != nil {
		return err
	}

	for _, name := range names {
		// Skip artifacts that do not match.
		if name_regex != nil && name_regex.FindString(name) == "" {
			continue
		}

		if *artifact_command_list_verbose_count == 0 {
			fmt.Println(name)
			continue
		}

		artifact, pres := repository.Get(ctx, config_obj, name)
		if !pres {
			return fmt.Errorf("Artifact %s not found", name)
		}

		fmt.Println(artifact.Raw)

		if *artifact_command_list_verbose_count <= 1 {
			continue
		}

		launcher, err := services.GetLauncher(config_obj)
		if err != nil {
			return err
		}

		request, err := launcher.CompileCollectorArgs(
			sm.Ctx, config_obj, acl_managers.NullACLManager{}, repository,
			services.CompilerOptions{
				DisablePrecondition: true,
			},
			&flows_proto.ArtifactCollectorArgs{
				Artifacts: []string{artifact.Name},
			})
		if err != nil {
			return fmt.Errorf("Unable to compile artifact: %w", err)
		}

		res, err := yaml.Marshal(request)
		if err != nil {
			return fmt.Errorf("Unable to encode artifact: %w", err)
		}

		fmt.Printf("VQLCollectorArgs %s:\n***********\n%v\n",
			artifact.Name, string(res))
	}
	return nil
}

func doArtifactCLIHelp(
	ctx context.Context,
	config_obj *config_proto.Config,
	manager services.RepositoryManager, artifact_name string) error {
	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}
	artifact, pres := repository.Get(ctx, config_obj, artifact_name)
	if !pres {
		return utils.Wrap(utils.NotFoundError,
			"Unknown artifact %v", artifact_name)
	}

	parse_context, err := app.ParseContext(
		[]string{"artifacts", "collect", artifact.Name})
	if err != nil {
		return err
	}
	app.UsageForContextWithTemplate(
		parse_context, 2, fmt.Sprintf(`
usage: {{.App.Name}}[<common flags> ...] -r %v [<artifact params> ...]:

Common Flags:
{{with .Context.Flags|FlagsToTwoColumns}}{{FormatTwoColumnsWithIndent . 4 2}}{{end}}
Artifact Parameters:
`, artifact.Name))

	for _, param := range artifact.Parameters {
		desc := strings.TrimSpace(param.Description)
		type_str := ""
		if param.Type != "" {
			type_str = fmt.Sprintf(" [%v] ", param.Type)
			switch param.Type {
			case "bool":
				desc += "\n\nValid values: Y / N"
			case "multichoice":
				desc += `

NOTE: Provide one or more of the following separated by ,
Valid values one or more of: ` + strings.Join(param.Choices, ", ")

			case "regex":
				desc += "\n\nNOTE: Be careful to escape regex backslashes from the shell!"
			}
		}

		desc = utils.Indent(utils.WrapString(desc, 100), 10) + "\n"

		if param.Default != "" {
			fmt.Printf(" --%v%v\n   default: '%v'\n%v",
				param.Name, type_str, param.Default, desc)
		} else {
			fmt.Printf(" --%v%v\n%v",
				param.Name, type_str, desc)
		}
	}
	return nil
}

func maybeAddDefinitionsDirectory(config_obj *config_proto.Config) error {
	if *artifact_definitions_dir != "" {
		if config_obj.Defaults == nil {
			config_obj.Defaults = &config_proto.Defaults{}
		}

		config_obj.Defaults.ArtifactDefinitionsDirectories = append(
			config_obj.Defaults.ArtifactDefinitionsDirectories,
			*artifact_definitions_dir)
	}
	return nil
}

// Simplify parsing of some parameter types. Since typing complicated
// JSON escapes on the command line may be complicated (especially on
// Windows) we also support some simpler types here
func parseArtifactType(param_type string, param string) string {
	switch param_type {
	case "multichoice", "json_array":
		var res []string
		err := json.Unmarshal([]byte(param), &res)
		if err != nil {
			// As an alternative for multi choice we allow items to be
			// separated by comma.
			for _, part := range strings.Split(param, ",") {
				res = append(res, strings.TrimSpace(part))
			}
			return json.MustMarshalString(res)
		}
	}
	return param
}

func getParameterType(
	ctx context.Context,
	config_obj *config_proto.Config,
	repository services.Repository,
	artifact_name, param_name string) (string, error) {
	artifact, pres := repository.Get(ctx, config_obj, artifact_name)
	if !pres {
		return "", utils.Wrap(utils.NotFoundError,
			"Unknown artifact %v", artifact_name)
	}

	for _, p := range artifact.Parameters {
		if p.Name == param_name {
			return p.Type, nil
		}
	}

	return "", utils.Wrap(utils.NotFoundError,
		"Parameter %v not known for artifact %v",
		param_name, artifact_name)
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case artifact_command_list.FullCommand():
			FatalIfError(artifact_command_list, doArtifactList)

		case artifact_command_show.FullCommand():
			FatalIfError(artifact_command_show, doArtifactShow)

		case artifact_command_collect.FullCommand():
			FatalIfError(artifact_command_collect, doArtifactCollect)

		default:
			return false
		}
		return true
	})
}
