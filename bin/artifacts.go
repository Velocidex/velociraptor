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
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/server"
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
		"name", "Name to show.").
		HintAction(listArtifactsHint).String()

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

	artifact_command_collect_report = artifact_command_collect.Flag(
		"report", "When specified we create a report html file.").
		Default("").String()

	artifact_command_collect_output_password = artifact_command_collect.Flag(
		"password", "When specified we encrypt zip file with this password.").
		Default("").String()

	artifact_command_collect_format = artifact_command_collect.Flag(
		"format", "Output format to use  (text,json,csv,jsonl).").
		Default("json").Enum("text", "json", "csv", "jsonl")

	artifact_command_collect_name = artifact_command_collect.Arg(
		"artifact_name", "The artifact name to collect.").
		Required().HintAction(listArtifactsHint).Strings()

	artifact_command_collect_args = artifact_command_collect.Flag(
		"args", "Artifact args.").Strings()
)

func listArtifactsHint() []string {
	config_obj := config.GetDefaultConfig()
	result := []string{}

	repository, err := artifacts.GetGlobalRepository(config_obj)
	if err != nil {
		return result
	}
	result = append(result, repository.List()...)
	return result
}

func getRepository(config_obj *config_proto.Config) (*artifacts.Repository, error) {
	repository, err := server.GetGlobalRepository(config_obj)
	kingpin.FatalIfError(err, "Artifact GetGlobalRepository ")
	if *artifact_definitions_dir != "" {
		logging.GetLogger(config_obj, &logging.ToolComponent).
			Info("Loading artifacts from %s",
				*artifact_definitions_dir)
		_, err := repository.LoadDirectory(*artifact_definitions_dir)
		if err != nil {
			logging.GetLogger(config_obj, &logging.ToolComponent).
				Error("Artifact LoadDirectory ", err)
			return nil, err
		}
	}

	return repository, nil
}

func printParameters(artifacts []string, repository *artifacts.Repository) {
	for _, name := range artifacts {
		artifact, _ := repository.Get(name)

		fmt.Printf("Parameters for artifact %s\n", artifact.Name)
		for _, arg := range artifact.Parameters {
			truncate_len := 80
			default_str := arg.Default

			if default_str != "" {
				if len(default_str) > truncate_len {
					default_str = default_str[:truncate_len] + "..."
				}

				default_str = "( " + default_str + " )"
			}

			descr_str := arg.Description
			if len(descr_str) > truncate_len {
				descr_str = descr_str[:truncate_len] + "..."
			}

			fmt.Printf("%s: %s %s\n", arg.Name, descr_str,
				default_str)
		}

		fmt.Printf("\n\n")
	}
}

func doArtifactCollect() {
	config_obj, err := DefaultConfigLoader.WithNullLoader().LoadAndValidate()
	kingpin.FatalIfError(err, "Load Config ")

	_, err = getRepository(config_obj)
	kingpin.FatalIfError(err, "Loading extra artifacts")

	now := time.Now()
	defer func() {
		logging.GetLogger(config_obj, &logging.ToolComponent).
			Info("Collection completed in %v Seconds",
				time.Now().Unix()-now.Unix())

	}()

	collect_args := ordereddict.NewDict()
	for _, item := range *artifact_command_collect_args {
		parts := strings.SplitN(item, "=", 2)
		arg_name := parts[0]

		if len(parts) < 2 {
			collect_args.Set(arg_name, "Y")
		} else {
			collect_args.Set(arg_name, parts[1])
		}
	}

	scope := artifacts.ScopeBuilder{
		Config:     config_obj,
		ACLManager: vql_subsystem.NullACLManager{},
		Logger:     log.New(&LogWriter{config_obj}, " ", 0),
		Env: ordereddict.NewDict().
			Set("Artifacts", *artifact_command_collect_name).
			Set("Output", *artifact_command_collect_output).
			Set("Password", *artifact_command_collect_output_password).
			Set("Report", *artifact_command_collect_report).
			Set("Args", collect_args).
			Set("Format", *artifact_command_collect_format),
	}.Build()
	defer scope.Close()

	if *trace_vql_flag {
		scope.Tracer = logging.NewPlainLogger(config_obj,
			&logging.ToolComponent)
	}

	ctx := InstallSignalHandler(scope)
	sm := services.NewServiceManager(ctx, config_obj)
	defer sm.Close()

	err = startEssentialServices(config_obj, sm)
	kingpin.FatalIfError(err, "Starting services.")

	query := `
  SELECT * FROM collect(artifacts=Artifacts, output=Output, report=Report,
                        password=Password, args=Args, format=Format)`
	eval_local_query(config_obj, *artifact_command_collect_format, query, scope)
}

func getFilterRegEx(pattern string) (*regexp.Regexp, error) {
	pattern = strings.Replace(pattern, "*", ".*", -1)
	pattern = "^" + pattern + "$"
	return regexp.Compile(pattern)
}

func doArtifactShow() {
	config_obj, err := DefaultConfigLoader.WithNullLoader().LoadAndValidate()
	kingpin.FatalIfError(err, "Load Config ")
	repository, err := getRepository(config_obj)
	kingpin.FatalIfError(err, "Loading extra artifacts")

	artifact, pres := repository.Get(*artifact_command_show_name)
	if !pres {
		kingpin.Fatalf("Artifact %s not found",
			*artifact_command_show_name)
	}

	fmt.Println(artifact.Raw)
}

func doArtifactList() {
	config_obj, err := DefaultConfigLoader.WithNullLoader().LoadAndValidate()
	kingpin.FatalIfError(err, "Load Config ")

	repository, err := getRepository(config_obj)
	kingpin.FatalIfError(err, "Loading extra artifacts")

	var name_regex *regexp.Regexp
	if *artifact_command_list_name != "" {
		re, err := getFilterRegEx(*artifact_command_list_name)
		kingpin.FatalIfError(err, "Artifact name regex not valid")

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

		artifact, pres := repository.Get(name)
		if !pres {
			kingpin.Fatalf("Artifact %s not found", name)
		}

		res, err := yaml.Marshal(artifact)
		kingpin.FatalIfError(err, "Unable to encode artifact.")

		fmt.Println(artifact.Raw)

		if *artifact_command_list_verbose_count <= 1 {
			continue
		}

		request := &actions_proto.VQLCollectorArgs{}
		err = repository.Compile(artifact, request)
		kingpin.FatalIfError(err, "Unable to compile artifact.")

		res, err = yaml.Marshal(request)
		kingpin.FatalIfError(err, "Unable to encode artifact.")

		fmt.Printf("VQLCollectorArgs %s:\n***********\n%v\n",
			artifact.Name, string(res))
	}
}

func load_config_artifacts(config_obj *config_proto.Config) error {
	if config_obj.Autoexec == nil {
		return nil
	}

	repository, err := getRepository(config_obj)
	if err != nil {
		return err
	}

	for _, definition := range config_obj.Autoexec.ArtifactDefinitions {
		definition.Raw = ""
		serialized, err := yaml.Marshal(definition)
		if err != nil {
			return err
		}

		// Add the raw definition for inspection.
		definition.Raw = string(serialized)

		_, err = repository.LoadProto(definition, true /* validate */)
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
			doArtifactList()

		case artifact_command_show.FullCommand():
			doArtifactShow()

		case artifact_command_collect.FullCommand():
			doArtifactCollect()

		default:
			return false
		}
		return true
	})
}
