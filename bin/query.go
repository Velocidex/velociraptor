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
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync"

	"github.com/Velocidex/ordereddict"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
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

	env_map = app.Flag("env", "Environment for the query.").
		StringMap()

	max_wait = app.Flag("max_wait", "Maximum time to queue results.").
			Default("10").Int()

	explain        = app.Command("explain", "Explain the output from a plugin")
	explain_plugin = explain.Arg("plugin", "Plugin to explain").Required().String()
)

func outputJSON(ctx context.Context,
	scope *vfilter.Scope,
	vql *vfilter.VQL,
	out io.Writer) {
	result_chan := vfilter.GetResponseChannel(vql, ctx, scope, 10, *max_wait)
	for {
		result, ok := <-result_chan
		if !ok {
			return
		}
		out.Write(result.Payload)
	}
}

func outputJSONL(ctx context.Context,
	scope *vfilter.Scope,
	vql *vfilter.VQL,
	out io.Writer) {
	result_chan := vfilter.GetResponseChannel(vql, ctx, scope, 10, *max_wait)
rows:
	for {
		result, ok := <-result_chan
		if !ok {
			return
		}

		result_array := []json.RawMessage{}
		err := json.Unmarshal(result.Payload, &result_array)
		if err != nil {
			continue rows
		}

		for _, item := range result_array {
			// Decode the row into an ordered dict to maintain ordering.
			row := ordereddict.NewDict()
			err = json.Unmarshal(item, row)
			if err != nil {
				continue
			}

			// Re-serialize it as compact json.
			serialized, err := json.Marshal(row)
			if err != nil {
				continue
			}

			out.Write(serialized)

			// Separate lines with \n
			out.Write([]byte("\n"))
		}
	}
}

func outputCSV(ctx context.Context,
	scope *vfilter.Scope,
	vql *vfilter.VQL,
	out io.Writer) {
	result_chan := vfilter.GetResponseChannel(vql, ctx, scope, 10, *max_wait)

	csv_writer := csv.GetCSVAppender(
		scope, &StdoutWrapper{out}, true /* write_headers */)
	defer csv_writer.Close()

	for result := range result_chan {
		payload := []map[string]interface{}{}
		err := json.Unmarshal(result.Payload, &payload)
		kingpin.FatalIfError(err, "outputCSV")

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

}

func doRemoteQuery(
	config_obj *config_proto.Config, format string,
	queries []string, env *ordereddict.Dict) {
	ctx := context.Background()
	client, closer, err := grpc_client.Factory.GetAPIClient(ctx, config_obj)
	kingpin.FatalIfError(err, "GetAPIClient")
	defer closer()

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
	kingpin.FatalIfError(err, "GetAPIClient")

	for {
		response, err := stream.Recv()
		if response == nil && err == io.EOF {
			break
		}
		kingpin.FatalIfError(err, "GetAPIClient")

		if response.Log != "" {
			logger.Info(response.Log)
			continue
		}

		rows, err := utils.ParseJsonToDicts([]byte(response.Response))
		kingpin.FatalIfError(err, "GetAPIClient")

		switch format {
		case "json":
			fmt.Println(response.Response)

		case "jsonl":
			for _, row := range rows {
				serialized, err := json.Marshal(row)
				if err == nil {
					fmt.Println(string(serialized))
				}
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
}

func doQuery() {
	config_obj, err := APIConfigLoader.WithNullLoader().LoadAndValidate()
	kingpin.FatalIfError(err, "Load Config")

	env := ordereddict.NewDict()
	for k, v := range *env_map {
		env.Set(k, v)
	}

	if config_obj.ApiConfig != nil && config_obj.ApiConfig.Name != "" {
		logging.GetLogger(config_obj, &logging.ToolComponent).
			Info("API Client configuration loaded - will make gRPC connection.")
		doRemoteQuery(config_obj, *format, *queries, env)
		return
	}

	// Try to start essential services in case they are needed. It
	// is not an error if we can not.
	_ = services.StartJournalService(config_obj)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg := &sync.WaitGroup{}
	defer wg.Wait()

	_ = services.StartNotificationService(ctx, wg, config_obj)

	repository, err := artifacts.GetGlobalRepository(config_obj)
	kingpin.FatalIfError(err, "Artifact GetGlobalRepository ")

	if *artifact_definitions_dir != "" {
		repository.LoadDirectory(*artifact_definitions_dir)
	}

	builder := artifacts.ScopeBuilder{
		Config:     config_obj,
		ACLManager: vql_subsystem.NullACLManager{},
		Logger:     log.New(&LogWriter{config_obj}, "Velociraptor: ", log.Lshortfile),
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

	scope := builder.Build()
	defer scope.Close()

	// Install throttler into the scope.
	vfilter.InstallThrottler(scope, vfilter.NewTimeThrottler(float64(*rate)))

	ctx = InstallSignalHandler(scope)

	if *trace_vql_flag {
		scope.Tracer = log.New(os.Stderr, "VQL Trace: ", log.Lshortfile)
	}
	for _, query := range *queries {
		statements, err := vfilter.MultiParse(query)
		kingpin.FatalIfError(err, "Unable to parse VQL Query")

		for _, vql := range statements {
			switch *format {
			case "text":
				table := reporting.EvalQueryToTable(ctx, scope, vql, os.Stdout)
				table.Render()
			case "json":
				outputJSON(ctx, scope, vql, os.Stdout)

			case "jsonl":
				outputJSONL(ctx, scope, vql, os.Stdout)

			case "csv":
				outputCSV(ctx, scope, vql, os.Stdout)
			}
		}
	}
}

func doExplain(plugin string) {
	result := ordereddict.NewDict()
	type_map := vfilter.NewTypeMap()
	scope := vql_subsystem.MakeScope()
	defer scope.Close()

	pslist_info, pres := scope.Info(type_map, plugin)
	if pres {
		result.Set(plugin+"_info", pslist_info)
		result.Set("type_map", type_map)
	}

	s, err := json.MarshalIndent(result, "", " ")
	if err == nil {
		os.Stdout.Write(s)
	}
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case explain.FullCommand():
			doExplain(*explain_plugin)

		case query.FullCommand():
			doQuery()

		default:
			return false
		}
		return true
	})
}
