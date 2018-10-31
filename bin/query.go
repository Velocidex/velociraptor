package main

import (
	"context"
	"encoding/json"
	"log"
	"os"

	"github.com/olekukonko/tablewriter"
	"gopkg.in/alecthomas/kingpin.v2"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	config "www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vql_networking "www.velocidex.com/golang/velociraptor/vql/networking"
	"www.velocidex.com/golang/vfilter"
)

var (
	// Command line interface for VQL commands.
	query   = app.Command("query", "Run a VQL query")
	queries = query.Arg("queries", "The VQL Query to run.").
		Required().Strings()
	format = query.Flag("format", "Output format to use.").
		Default("json").Enum("text", "json")
	dump_dir = query.Flag("dump_dir", "Directory to dump output files.").
			Default(".").String()

	max_wait = query.Flag("max_wait", "Maximum time to queue results.").
			Default("10").Int()

	explain        = app.Command("explain", "Explain the output from a plugin")
	explain_plugin = explain.Arg("plugin", "Plugin to explain").Required().String()
)

func outputJSON(scope *vfilter.Scope, vql *vfilter.VQL) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result_chan := vfilter.GetResponseChannel(vql, ctx, scope, 10, *max_wait)
	for {
		result, ok := <-result_chan
		if !ok {
			return
		}
		os.Stdout.Write(result.Payload)
	}
}

func evalQueryToTable(scope *vfilter.Scope, vql *vfilter.VQL) *tablewriter.Table {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	output_chan := vql.Eval(ctx, scope)
	table := tablewriter.NewWriter(os.Stdout)

	columns := vql.Columns(scope)
	table.SetHeader(*columns)
	table.SetCaption(true, vql.ToString(scope))
	table.SetAutoFormatHeaders(false)
	table.SetAutoWrapText(false)

	for {
		row, ok := <-output_chan
		if !ok {
			return table
		}
		string_row := []string{}
		if len(*columns) == 0 {
			members := scope.GetMembers(row)
			table.SetHeader(members)
			columns = &members
		}

		for _, key := range *columns {
			cell := ""
			value, pres := scope.Associative(row, key)
			if pres && !utils.IsNil(value) {
				cell = utils.Stringify(value, scope)
			}
			string_row = append(string_row, cell)
		}

		table.Append(string_row)
	}
}

func doQuery() {
	config_obj, err := config.LoadConfig(*config_path)
	if err != nil {
		config_obj = config.GetDefaultConfig()
	}
	repository, err := artifacts.GetGlobalRepository(config_obj)
	kingpin.FatalIfError(err, "Artifact GetGlobalRepository ")
	repository.LoadDirectory(*artifact_definitions_dir)

	env := vfilter.NewDict().
		Set("config", config_obj.Client).
		Set("server_config", config_obj).
		Set("$uploader", &vql_networking.FileBasedUploader{
			UploadDir: *dump_dir,
		})
	scope := artifacts.MakeScope(repository).AppendVars(env)

	scope.Logger = log.New(os.Stderr, "velociraptor: ", log.Lshortfile)
	for _, query := range *queries {
		vql, err := vfilter.Parse(query)
		if err != nil {
			kingpin.FatalIfError(err, "Unable to parse VQL Query")
		}

		switch *format {
		case "text":
			table := evalQueryToTable(scope, vql)
			table.Render()
		case "json":
			outputJSON(scope, vql)
		}
	}
}

func doExplain(plugin string) {
	result := vfilter.NewDict()
	type_map := vfilter.NewTypeMap()
	scope := vql_subsystem.MakeScope()
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
