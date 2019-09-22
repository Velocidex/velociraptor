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
	"os"
	"regexp"
	"strings"

	"github.com/Velocidex/yaml"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/server"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	artifact_command = app.Command(
		"artifacts", "Process artifact definitions.")

	artifact_command_list = artifact_command.Command(
		"list", "Print all artifacts")

	artifact_command_list_name = artifact_command_list.Arg(
		"regex", "Regex of names to match.").
		HintAction(listArtifacts).String()

	artifact_command_list_verbose_count = artifact_command_list.Flag(
		"details", "Show more details (Use -d -dd for even more)").
		Short('d').Counter()

	artifact_command_collect = artifact_command.Command(
		"collect", "Collect all artifacts")

	artifact_command_collect_output = artifact_command_collect.Flag(
		"output", "When specified we create a zip file and "+
			"store all output in it.").
		Default("").String()

	artifact_command_collect_format = artifact_command_collect.Flag(
		"format", "Output format to use.").
		Default("json").Enum("text", "json", "csv")

	artifact_command_collect_details = artifact_command_collect.Flag(
		"details", "Show more details (Use -d -dd for even more)").
		Short('d').Counter()

	artifact_command_collect_name = artifact_command_collect.Arg(
		"artifact_name", "The artifact name to collect.").
		Required().HintAction(listArtifacts).Strings()

	artifact_command_collect_args = artifact_command_collect.Flag(
		"args", "Artifact args.").Strings()
)

func listArtifacts() []string {
	result := []string{}
	config_obj := get_config_or_default()
	repository, err := artifacts.GetGlobalRepository(config_obj)
	if err != nil {
		return result
	}
	result = append(result, repository.List()...)
	return result
}

func collectArtifact(
	config_obj *config_proto.Config,
	repository *artifacts.Repository,
	artifact_name string,
	request *actions_proto.VQLCollectorArgs) {
	env := vfilter.NewDict().
		Set("config", config_obj.Client).
		Set("server_config", config_obj).
		Set(vql_subsystem.CACHE_VAR, vql_subsystem.NewScopeCache())

	for _, request_env := range request.Env {
		env.Set(request_env.Key, request_env.Value)
	}

	scope := artifacts.MakeScope(repository).AppendVars(env)
	defer scope.Close()

	scope.Logger = logging.NewPlainLogger(config_obj,
		&logging.ToolComponent)

	ctx := InstallSignalHandler(scope)

	for _, query := range request.Query {
		vql, err := vfilter.Parse(query.VQL)
		kingpin.FatalIfError(err, "Parse VQL")

		switch *artifact_command_collect_format {
		case "text":
			var rows []vfilter.Row
			for row := range vql.Eval(ctx, scope) {
				rows = append(rows, row)
			}

			if *artifact_command_collect_details > 0 {
				if query.Name != "" {
					fmt.Printf("# %s\n\n", query.Name)
				}
				if query.Description != "" {
					fmt.Printf("%s\n\n", reporting.FormatDescription(
						config_obj, query.Description, rows))
				}
			}

			// Queries without a name do not produce
			// interesting results.
			table := reporting.OutputRowsToTable(scope, rows, os.Stdout)
			if query.Name == "" {
				continue
			}
			table.SetCaption(true, query.Name)
			if table.NumLines() > 0 {
				table.Render()
			}
			fmt.Println("")

		case "json":
			outputJSON(ctx, scope, vql, os.Stdout)

		case "csv":
			outputCSV(ctx, scope, vql, os.Stdout)
		}
	}
}

func collectArtifactToContainer(
	config_obj *config_proto.Config,
	repository *artifacts.Repository,
	artifact_name string,
	container *reporting.Container,
	request *actions_proto.VQLCollectorArgs) {
	env := vfilter.NewDict().
		Set("config", config_obj.Client).
		Set("server_config", config_obj).
		Set(vql_subsystem.CACHE_VAR, vql_subsystem.NewScopeCache())

	// Any uploads go into the container.
	env.Set("$uploader", container)

	for _, request_env := range request.Env {
		env.Set(request_env.Key, request_env.Value)
	}

	scope := artifacts.MakeScope(repository).AppendVars(env)
	defer scope.Close()

	scope.Logger = logging.NewPlainLogger(config_obj,
		&logging.ToolComponent)

	ctx := InstallSignalHandler(scope)

	for _, query := range request.Query {
		vql, err := vfilter.Parse(query.VQL)
		kingpin.FatalIfError(err, "Parse VQL")

		// Store query output in the container.
		err = container.StoreArtifact(config_obj, ctx, scope, vql, query)
		kingpin.FatalIfError(err, "container.StoreArtifact")

		if query.Name != "" {
			logging.GetLogger(config_obj, &logging.ToolComponent).
				Info("Collected %s", query.Name)
		}
	}
}

func getRepository(config_obj *config_proto.Config) *artifacts.Repository {
	repository, err := server.GetGlobalRepository(config_obj)
	kingpin.FatalIfError(err, "Artifact GetGlobalRepository ")
	if *artifact_definitions_dir != "" {
		logging.GetLogger(config_obj, &logging.ToolComponent).
			Info("Loading artifacts from %s",
				*artifact_definitions_dir)
		_, err := repository.LoadDirectory(*artifact_definitions_dir)
		if err != nil {
			logging.GetLogger(config_obj, &logging.ToolComponent).
				Error("Artifact LoadDirectory", err)
		}
	}

	return repository
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

// Check if the user specified an unknown parameter.
func valid_parameter(param_name string, repository *artifacts.Repository) bool {
	for _, name := range *artifact_command_collect_name {
		artifact, _ := repository.Get(name)
		for _, param := range artifact.Parameters {
			if param.Name == param_name {
				return true
			}
		}
	}

	return false
}

func doArtifactCollect() {
	config_obj := get_config_or_default()
	repository := getRepository(config_obj)

	var container *reporting.Container
	var err error

	if *artifact_command_collect_output != "" {
		// Create an output container.
		container, err = reporting.NewContainer(*artifact_command_collect_output)
		kingpin.FatalIfError(err, "Can not create output container")
		defer container.Close()
	}

	for _, name := range *artifact_command_collect_name {
		artifact, pres := repository.Get(name)
		if !pres {
			kingpin.Fatalf("Artifact %v not known.", name)
		}

		request := &actions_proto.VQLCollectorArgs{
			MaxWait: uint64(*max_wait),
		}

		err := repository.Compile(artifact, request)
		kingpin.FatalIfError(
			err, fmt.Sprintf("Unable to compile artifact %s.",
				name))

		if env_map != nil {
			for k, v := range *env_map {
				if !valid_parameter(k, repository) {
					kingpin.Fatalf(
						"Param %v not known for %s.",
						k, strings.Join(*artifact_command_collect_name, ","))
				}
				request.Env = append(
					request.Env, &actions_proto.VQLEnv{
						Key: k, Value: v,
					})
			}
		}

		for _, item := range *artifact_command_collect_args {
			parts := strings.SplitN(item, "=", 2)
			arg_name := parts[0]
			var arg string

			if len(parts) < 2 {
				arg = "Y"
			} else {
				arg = parts[1]
			}
			if !valid_parameter(arg_name, repository) {
				printParameters(*artifact_command_collect_name,
					repository)
				kingpin.Fatalf("Param %v not known for any artifacts.",
					arg_name)
			}
			request.Env = append(
				request.Env, &actions_proto.VQLEnv{
					Key: arg_name, Value: arg,
				})
		}

		if *artifact_command_collect_output == "" {
			collectArtifact(config_obj, repository, name, request)
		} else {
			collectArtifactToContainer(
				config_obj, repository, name, container, request)
		}
	}
}

func getFilterRegEx(pattern string) (*regexp.Regexp, error) {
	pattern = strings.Replace(pattern, "*", ".*", -1)
	pattern = "^" + pattern + "$"
	return regexp.Compile(pattern)
}

func doArtifactList() {
	config_obj := get_config_or_default()
	repository := getRepository(config_obj)

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

		fmt.Printf("Definition %s:\n***********\n%v\n",
			artifact.Name, string(res))

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

func load_config_artifacts(config_obj *config_proto.Config) {
	if config_obj.Autoexec == nil {
		return
	}
	repository := getRepository(config_obj)
	for _, definition := range config_obj.Autoexec.ArtifactDefinitions {
		_, err := repository.LoadYaml(definition, true /* validate */)
		kingpin.FatalIfError(err, "Unable to parse config artifact")
	}
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case artifact_command_list.FullCommand():
			doArtifactList()

		case artifact_command_collect.FullCommand():
			doArtifactCollect()

		default:
			return false
		}
		return true
	})
}
