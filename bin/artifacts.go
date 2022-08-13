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
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/yaml/v2"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/executor"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/startup"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
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

	// Artifacts specified with the --definitions flag take priority
	// and can override built in artifacts
	if *artifact_definitions_dir != "" {
		logging.GetLogger(config_obj, &logging.ToolComponent).
			Info("Loading artifacts from %s", *artifact_definitions_dir)
		_, err := repository.LoadDirectory(
			config_obj, *artifact_definitions_dir, true /* override_builtins */)
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

	ctx, top_cancel := install_sig_handler()
	defer top_cancel()

	sm, err := startup.StartToolServices(ctx, config_obj)
	defer sm.Close()

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

			if len(parts) < 2 {
				collect_args.Set(arg_name, "Y")
			} else {
				collect_args.Set(arg_name, parts[1])
			}
		}

		spec.Set(name, collect_args)
	}

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return err
	}

	logger := log.New(&LogWriter{config_obj}, "", 0)

	scope := manager.BuildScope(services.ScopeBuilder{
		Config:     config_obj,
		ACLManager: acl_managers.NullACLManager{},
		Logger:     logger,
		Env: ordereddict.NewDict().
			Set("Artifacts", *artifact_command_collect_names).
			Set("Output", *artifact_command_collect_output).
			Set("Level", *artifact_command_collect_output_compression).
			Set("Password", *artifact_command_collect_output_password).
			Set("Report", *artifact_command_collect_report).
			Set("Template", *artifact_command_collect_report_template).
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
	scope.AddDestructor(func() {
		sm.Wg.Done()
	})

	if *artifact_command_collect_hardmemory > 0 {
		scope.Log("Installing hard memory limit of %v bytes",
			*artifact_command_collect_hardmemory)
		Nanny := &executor.NannyService{
			MaxMemoryHardLimit: *artifact_command_collect_hardmemory,
			Logger: logging.GetLogger(
				config_obj, &logging.ToolComponent),
			OnExit: sm.Close,
		}

		// Keep the nanny running after the query is done so it can
		// hard kill the process if cancellation is not enough.
		Nanny.Start(ctx, &sync.WaitGroup{})
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
                        timeout=Timeout, progress_timeout=ProgressTimeout,
                        cpu_limit=CpuLimit,
                        password=Password, args=Args, format=Format)`
	return eval_local_query(
		sm.Ctx, config_obj,
		*artifact_command_collect_format, query, scope)
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

		artifact, pres := repository.Get(config_obj, name)
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
