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
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/yaml/v2"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

var (
	artifact_command = app.Command(
		"artifacts", "Process artifact definitions.")

	artifact_command_list = artifact_command.Command(
		"list", "Print all artifacts")

	artifact_command_show = artifact_command.Command(
		"show", "Show an artifact")

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

	artifact_command_collect_output_compression = artifact_command_collect.Flag(
		"output_level", "Compression level for zip output.").
		Default("5").Int64()

	artifact_command_collect_report = artifact_command_collect.Flag(
		"report", "When specified we create a report html file.").
		Default("").String()

	artifact_command_collect_report_template = artifact_command_collect.Flag(
		"report_template", "Use this artifact to provide the report template.").
		Default("").String()

	artificat_command_collect_admin_flag = artifact_command_collect.Flag(
		"require_admin", "Ensure the user is an admin").Bool()

	artifact_command_collect_output_password = artifact_command_collect.Flag(
		"password", "When specified we encrypt zip file with this password.").
		Default("").String()

	artifact_command_collect_format = artifact_command_collect.Flag(
		"format", "Output format to use  (text,json,csv,jsonl).").
		Default("json").Enum("text", "json", "csv", "jsonl")

	artifact_command_collect_names = artifact_command_collect.Arg(
		"artifact_name", "The artifact name to collect.").
		Required().HintAction(listArtifactsHint).Strings()

	artifact_command_collect_args = artifact_command_collect.Flag(
		"args", "Artifact args.").Strings()
)

func listArtifactsHint() []string {
	config_obj := config.GetDefaultConfig()
	result := []string{}

	repository, err := getRepository(config_obj)
	if err != nil {
		return result
	}
	result = append(result, repository.List()...)
	return result
}

func getRepository(config_obj *config_proto.Config) (services.Repository, error) {
	manager, err := services.GetRepositoryManager()
	if err != nil {
		return nil, err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}

	if *artifact_definitions_dir != "" {
		logging.GetLogger(config_obj, &logging.ToolComponent).
			Info("Loading artifacts from %s",
				*artifact_definitions_dir)
		_, err := repository.LoadDirectory(config_obj, *artifact_definitions_dir)
		if err != nil {
			logging.GetLogger(config_obj, &logging.ToolComponent).
				Error("Artifact LoadDirectory: %v ", err)
			return nil, err
		}
	}

	return repository, nil
}

func doArtifactCollect() error {
	err := checkAdmin()
	if err != nil {
		return err
	}

	config_obj, err := makeDefaultConfigLoader().
		WithNullLoader().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to create config: %w", err)
	}

	sm, err := startEssentialServices(config_obj)
	if err != nil {
		return fmt.Errorf("Can't load service: %w", err)
	}
	defer sm.Close()

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

			if len(parts) < 2 {
				collect_args.Set(arg_name, "Y")
			} else {
				collect_args.Set(arg_name, parts[1])
			}
		}

		spec.Set(name, collect_args)
	}

	manager, err := services.GetRepositoryManager()
	if err != nil {
		return err
	}

	scope := manager.BuildScope(services.ScopeBuilder{
		Config:     config_obj,
		ACLManager: vql_subsystem.NullACLManager{},
		Logger:     log.New(&LogWriter{config_obj}, " ", 0),
		Env: ordereddict.NewDict().
			Set("Artifacts", *artifact_command_collect_names).
			Set("Output", *artifact_command_collect_output).
			Set("Level", *artifact_command_collect_output_compression).
			Set("Password", *artifact_command_collect_output_password).
			Set("Report", *artifact_command_collect_report).
			Set("Template", *artifact_command_collect_report_template).
			Set("Args", spec).
			Set("Format", *artifact_command_collect_format),
	})
	defer scope.Close()

	_, err = getRepository(config_obj)
	if err != nil {
		return err
	}

	now := time.Now()
	defer func() {
		logging.GetLogger(config_obj, &logging.ToolComponent).
			Info("Collection completed in %v Seconds",
				time.Now().Unix()-now.Unix())

	}()

	if *trace_vql_flag {
		scope.SetTracer(logging.NewPlainLogger(config_obj,
			&logging.ToolComponent))
	}

	query := `
  SELECT * FROM collect(artifacts=Artifacts, output=Output, report=Report,
                        level=Level, template=Template,
                        password=Password, args=Args, format=Format)`
	return eval_local_query(config_obj, *artifact_command_collect_format,
		query, scope)
}

func getFilterRegEx(pattern string) (*regexp.Regexp, error) {
	pattern = strings.Replace(pattern, "*", ".*", -1)
	pattern = "^" + pattern + "$"
	return regexp.Compile(pattern)
}

func doArtifactShow() error {
	config_obj, err := makeDefaultConfigLoader().
		WithNullLoader().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to create config: %w", err)
	}

	sm, err := startEssentialServices(config_obj)
	if err != nil {
		return fmt.Errorf("Can't load service: %w", err)
	}
	defer sm.Close()

	repository, err := getRepository(config_obj)
	if err != nil {
		return fmt.Errorf("Loading extra artifacts: %w", err)
	}

	artifact, pres := repository.Get(config_obj, *artifact_command_show_name)
	if !pres {
		return fmt.Errorf("Artifact %s not found",
			*artifact_command_show_name)
	}

	fmt.Println(artifact.Raw)
	return nil
}

func doArtifactList() error {
	config_obj, err := makeDefaultConfigLoader().
		WithNullLoader().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	sm, err := startEssentialServices(config_obj)
	if err != nil {
		return fmt.Errorf("Starting services: %w", err)
	}
	defer sm.Close()

	ctx, cancel := install_sig_handler()
	defer cancel()

	repository, err := getRepository(config_obj)
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

	for _, name := range repository.List() {
		// Skip artifacts that do not match.
		if name_regex != nil && name_regex.FindString(name) == "" {
			continue
		}

		if *artifact_command_list_verbose_count == 0 {
			fmt.Println(name)
			continue
		}

		artifact, pres := repository.Get(config_obj, name)
		if !pres {
			return fmt.Errorf("Artifact %s not found", name)
		}

		fmt.Println(artifact.Raw)

		if *artifact_command_list_verbose_count <= 1 {
			continue
		}

		launcher, err := services.GetLauncher()
		if err != nil {
			return err
		}

		request, err := launcher.CompileCollectorArgs(
			ctx, config_obj, vql_subsystem.NullACLManager{}, repository,
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

// Load any artifacts defined inside the config file.
func load_config_artifacts(config_obj *config_proto.Config) error {
	if config_obj.Autoexec == nil {
		return nil
	}

	repository, err := getRepository(config_obj)
	if err != nil {
		return err
	}

	for _, definition := range config_obj.Autoexec.ArtifactDefinitions {
		serialized, err := yaml.Marshal(definition)
		if err != nil {
			return err
		}

		_, err = repository.LoadYaml(string(serialized), true /* validate */)
		if err != nil {
			return err
		}
	}
	return nil
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
