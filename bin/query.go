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
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/Velocidex/ordereddict"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/json"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/startup"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	// Command line interface for VQL commands.
	query   = app.Command("query", "Run a VQL query")
	queries = query.Arg("queries", "The VQL Query to run.").
		Required().Strings()

	rate = app.Flag("ops_per_second", "Rate of execution").
		Default("1000000").Float64()
	format = query.Flag("format", "Output format to use (text,json,csv,jsonl).").
		Default("json").Enum("text", "json", "csv", "jsonl")

	dump_dir = query.Flag("dump_dir", "Directory to dump output files.").
			Default("").String()

	output_file = query.Flag("output", "A file to store the output.").
			Default("").String()

	env_map = query.Flag("env", "Environment for the query.").
		StringMap()

	max_wait = app.Flag("max_wait", "Maximum time to queue results.").
			Default("10").Int()

	scope_file = query.Flag("scope_file",
		"Load scope from here. Creates a new file if file not found").
		Default("").String()

	do_not_update = query.Flag("do_not_update_scope_file",
		"Do not update the scope file with the new scope").Bool()
)

func outputJSON(ctx context.Context,
	scope vfilter.Scope,
	vql *vfilter.VQL,
	out io.Writer) error {
	for result := range vfilter.GetResponseChannel(
		vql, ctx, scope,
		vql_subsystem.MarshalJsonIndent(scope),
		10, *max_wait) {
		_, err := out.Write(result.Payload)
		if err != nil {
			return err
		}
	}
	return nil
}

func outputJSONL(ctx context.Context,
	scope vfilter.Scope,
	vql *vfilter.VQL,
	out io.Writer) error {
	for result := range vfilter.GetResponseChannel(
		vql, ctx, scope,
		vql_subsystem.MarshalJsonl(scope),
		10, *max_wait) {
		_, err := out.Write(result.Payload)
		if err != nil {
			return err
		}
	}
	return nil
}

func outputCSV(ctx context.Context,
	scope vfilter.Scope,
	vql *vfilter.VQL,
	out io.Writer) error {
	result_chan := vfilter.GetResponseChannel(vql, ctx, scope,
		vql_subsystem.MarshalJson(scope),
		10, *max_wait)

	csv_writer := csv.GetCSVAppender(
		scope, &StdoutWrapper{out}, true /* write_headers */)
	defer csv_writer.Close()

	for result := range result_chan {
		payload := []map[string]interface{}{}
		err := json.Unmarshal(result.Payload, &payload)
		if err != nil {
			return err
		}

		for _, row := range payload {
			row_dict := ordereddict.NewDict()
			for _, column := range result.Columns {
				value, pres := row[column]
				if pres {
					row_dict.Set(column, value)
				}
			}

			csv_writer.Write(row_dict)
		}
	}
	return nil
}

func doRemoteQuery(
	config_obj *config_proto.Config, format string,
	queries []string, env *ordereddict.Dict) error {
	ctx := context.Background()
	client, closer, err := grpc_client.Factory.GetAPIClient(ctx, config_obj)
	if err != nil {
		return err
	}
	defer func() { _ = closer() }()

	logger := logging.GetLogger(config_obj, &logging.ToolComponent)

	request := &actions_proto.VQLCollectorArgs{
		MaxRow:  1000,
		MaxWait: 1,
	}

	if env != nil {
		for _, k := range env.Keys() {
			v, ok := env.GetString(k)
			if ok {
				request.Env = append(request.Env, &actions_proto.VQLEnv{
					Key: k, Value: v})
			}
		}
	}

	for _, query := range queries {
		request.Query = append(request.Query,
			&actions_proto.VQLRequest{VQL: query})
	}
	stream, err := client.Query(context.Background(), request)
	if err != nil {
		return err
	}

	for {
		response, err := stream.Recv()
		if response == nil && err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if response.Log != "" {
			logger.Info(response.Log)
			continue
		}

		json_response := response.Response
		if json_response == "" {
			json_response = response.JSONLResponse
		}

		rows, err := utils.ParseJsonToDicts([]byte(json_response))
		if err != nil {
			return err
		}

		switch format {
		case "text":
			vfilter_rows := make([]vfilter.Row, 0, len(rows))
			for _, row := range rows {
				vfilter_rows = append(vfilter_rows, row)
			}

			scope := vql_subsystem.MakeScope()
			table := reporting.OutputRowsToTable(scope, vfilter_rows, os.Stdout)
			table.Render()

		case "json":
			fmt.Println(string(json.MustMarshalIndent(rows)))

		case "jsonl":
			for _, row := range rows {
				fmt.Println(json.MustMarshalString(row))
			}

		case "csv":
			scope := vql_subsystem.MakeScope()

			csv_writer := csv.GetCSVAppender(
				scope, &StdoutWrapper{os.Stdout}, true /* write_headers */)
			defer csv_writer.Close()

			for _, row := range rows {
				csv_writer.Write(row)
			}
		}
	}
	return nil
}

func startEssentialServices(config_obj *config_proto.Config) (
	*services.Service, error) {

	sm := services.NewServiceManager(context.Background(), config_obj)
	err := startup.StartupEssentialServices(sm)
	if err != nil {
		return nil, err
	}

	// Load any artifacts defined in the config file after all the
	// services are up.
	err = load_config_artifacts(config_obj)
	return sm, err
}

func doQuery() error {
	config_obj, err := APIConfigLoader.WithNullLoader().LoadAndValidate()
	if err != nil {
		return err
	}

	sm, err := startEssentialServices(config_obj)
	if err != nil {
		return fmt.Errorf("Starting services: %w", err)
	}
	defer sm.Close()

	env := ordereddict.NewDict()
	for k, v := range *env_map {
		env.Set(k, v)
	}

	if config_obj.ApiConfig != nil && config_obj.ApiConfig.Name != "" {
		logging.GetLogger(config_obj, &logging.ToolComponent).
			Info("API Client configuration loaded - will make gRPC connection.")
		return doRemoteQuery(config_obj, *format, *queries, env)
	}

	// Initialize the repository in case the artifacts use it
	_, err = getRepository(config_obj)
	if err != nil {
		return fmt.Errorf("Artifact GetGlobalRepository: %w ", err)
	}

	builder := services.ScopeBuilder{
		Config:     config_obj,
		ACLManager: vql_subsystem.NullACLManager{},
		Logger:     log.New(&LogWriter{config_obj}, "", 0),
		Env:        ordereddict.NewDict(),
	}

	if *run_as != "" {
		builder.ACLManager = vql_subsystem.NewServerACLManager(config_obj, *run_as)
	}

	// Configure an uploader if required.
	if *dump_dir != "" {
		builder.Uploader = &uploads.FileBasedUploader{
			UploadDir: *dump_dir,
		}
	}

	if env_map != nil {
		for k, v := range *env_map {
			builder.Env.Set(k, v)
		}
	}

	manager, err := services.GetRepositoryManager()
	if err != nil {
		return err
	}
	scope := manager.BuildScope(builder)
	defer scope.Close()

	if *scope_file != "" {
		scope, err = loadScopeFromFile(*scope_file, scope)
		if err != nil {
			return fmt.Errorf("loadScopeFromFile: %w", err)
		}

		// When the scope is destroyed store it in the file again.
		if !*do_not_update {
			scope.AddDestructor(func() {
				err := storeScopeInFile(*scope_file, scope)
				if err != nil {
					scope.Log("Storing scope in %v: %v",
						*scope_file, err)
				}
			})
		}

	}

	// Install throttler into the scope.
	vfilter.InstallThrottler(scope, vfilter.NewTimeThrottler(float64(*rate)))

	ctx := InstallSignalHandler(scope)

	out_fd := os.Stdout
	if *output_file != "" {
		out_fd, err = os.OpenFile(*output_file,
			os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			return err
		}
		defer out_fd.Close()
	}

	start_time := time.Now()
	defer func() {
		scope.Log("Completed query in %v", time.Now().Sub(start_time))
	}()

	if *trace_vql_flag {
		scope.SetTracer(log.New(os.Stderr, "VQL Trace: ", 0))
	}
	for _, query := range *queries {
		statements, err := vfilter.MultiParse(query)
		kingpin.FatalIfError(err, "Unable to parse VQL Query")

		for _, vql := range statements {
			switch *format {
			case "text":
				table := reporting.EvalQueryToTable(ctx, scope, vql, out_fd)
				table.Render()
			case "json":
				err = outputJSON(ctx, scope, vql, out_fd)
				if err != nil {
					return err
				}

			case "jsonl":
				err = outputJSONL(ctx, scope, vql, out_fd)
				if err != nil {
					return err
				}
			case "csv":
				err = outputCSV(ctx, scope, vql, out_fd)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case query.FullCommand():
			FatalIfError(query, doQuery)

		default:
			return false
		}
		return true
	})
}
