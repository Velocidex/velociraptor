package main

import (
	"fmt"
	"github.com/ghodss/yaml"
	"gopkg.in/alecthomas/kingpin.v2"
	"regexp"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	config "www.velocidex.com/golang/velociraptor/config"
	logging "www.velocidex.com/golang/velociraptor/logging"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	artifact_command = app.Command(
		"artifacts", "Process artifact definitions.")

	artifact_command_list = artifact_command.Command(
		"list", "Print all artifacts")

	artifact_command_list_name = artifact_command_list.Arg(
		"regex", "Regex of names to match.").String()

	artifact_command_collect = artifact_command.Command(
		"collect", "Collect all artifacts")

	artifact_command_collect_dump_dir = artifact_command_collect.Flag(
		"dump_dir", "Directory to dump output files.").
		Default(".").String()
	artifact_command_collect_format = artifact_command_collect.Flag(
		"format", "Output format to use.").
		Default("text").Enum("text", "json")
	artifact_command_collect_name = artifact_command_collect.Arg(
		"regex", "Regex of artifact names to collect.").
		Required().String()
)

func collectArtifact(
	config_obj *config.Config,
	artifact_name string,
	request *actions_proto.VQLCollectorArgs) {
	env := vfilter.NewDict().
		Set("config", config_obj.Client).
		Set("$uploader", &vql_subsystem.FileBasedUploader{
			*artifact_command_collect_dump_dir})

	for _, request_env := range request.Env {
		env.Set(request_env.Key, request_env.Value)
	}
	scope := vql_subsystem.MakeScope().AppendVars(env)
	scope.Logger = logging.NewPlainLogger(config_obj)
	for _, query := range request.Query {
		vql, err := vfilter.Parse(query.VQL)
		kingpin.FatalIfError(err, "Parse VQL")

		switch *artifact_command_collect_format {
		case "text":
			table := evalQueryToTable(scope, vql)
			table.SetCaption(true, "Artifact: "+artifact_name)
			if table.NumLines() > 0 {
				table.Render()
			}
		case "json":
			outputJSON(scope, vql)
		}
	}
}

func getRepository(config_obj *config.Config) *artifacts.Repository {
	repository, err := artifacts.GetGlobalRepository(config_obj)
	kingpin.FatalIfError(err, "Artifact GetGlobalRepository ")
	if *artifact_definitions_dir != "" {
		err := repository.LoadDirectory(*artifact_definitions_dir)
		if err != nil {
			logging.NewLogger(config_obj).Error("Artifact LoadDirectory", err)
		}
	}

	return repository
}

func doArtifactCollect() {
	config_obj := get_config_or_default()
	repository := getRepository(config_obj)
	var name_regex *regexp.Regexp
	if *artifact_command_collect_name != "" {
		re, err := regexp.Compile(*artifact_command_collect_name)
		kingpin.FatalIfError(err, "Artifact name regex not valid")

		name_regex = re
	}

	for _, name := range repository.List() {
		// Skip artifacts that do not match.
		if name_regex != nil && name_regex.FindString(name) == "" {
			continue
		}

		artifact, pres := repository.Get(name)
		if !pres {
			kingpin.Fatalf("Artifact not found")
		}

		request := &actions_proto.VQLCollectorArgs{}
		err := artifacts.Compile(artifact, request)
		kingpin.FatalIfError(
			err, fmt.Sprintf("Unable to compile artifact %s.", name))

		collectArtifact(config_obj, name, request)
	}
}

func doArtifactList() {
	config_obj := get_config_or_default()
	repository := getRepository(config_obj)

	var name_regex *regexp.Regexp
	if *artifact_command_list_name != "" {
		re, err := regexp.Compile(*artifact_command_list_name)
		kingpin.FatalIfError(err, "Artifact name regex not valid")

		name_regex = re
	}

	for _, name := range repository.List() {
		// Skip artifacts that do not match.
		if name_regex != nil && name_regex.FindString(name) == "" {
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

		request := &actions_proto.VQLCollectorArgs{}
		err = artifacts.Compile(artifact, request)
		kingpin.FatalIfError(err, "Unable to compile artifact.")

		res, err = yaml.Marshal(request)
		kingpin.FatalIfError(err, "Unable to encode artifact.")

		fmt.Printf("VQLCollectorArgs %s:\n***********\n%v\n",
			artifact.Name, string(res))
	}
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case "artifacts list":
			doArtifactList()
		case "artifacts collect":
			doArtifactCollect()
		default:
			return false
		}
		return true
	})
}
