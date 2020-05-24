package main

import (
	"log"
	"os"

	"github.com/Velocidex/ordereddict"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/reporting"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	csv_cmd        = app.Command("csv", "Convert a CSV file to another format")
	csv_cmd_filter = csv_cmd.Flag("where", "A WHERE condition for the query").String()
	csv_format     = csv_cmd.Flag("format", "Output format").
			Default("jsonl").Enum("text", "json", "jsonl")
	csv_cmd_files = csv_cmd.Arg("files", "CSV files to parse").Required().Strings()
)

func doCSV() {
	config_obj, err := DefaultConfigLoader.WithNullLoader().LoadAndValidate()
	kingpin.FatalIfError(err, "Load Config ")

	builder := artifacts.ScopeBuilder{
		Config:     config_obj,
		ACLManager: vql_subsystem.NullACLManager{},
		Logger:     log.New(os.Stderr, "velociraptor: ", log.Lshortfile),
		Env: ordereddict.NewDict().
			Set(vql_subsystem.ACL_MANAGER_VAR,
				vql_subsystem.NewRoleACLManager("administrator")).
			Set("Files", *csv_cmd_files),
	}

	scope := builder.Build()
	defer scope.Close()

	query := "SELECT * FROM parse_csv(filename=Files)"

	if *csv_cmd_filter != "" {
		query += " WHERE " + *csv_cmd_filter
	}

	vql, err := vfilter.Parse(query)
	kingpin.FatalIfError(err, "Unable to parse VQL Query")

	ctx := InstallSignalHandler(scope)

	switch *csv_format {
	case "text":
		table := reporting.EvalQueryToTable(ctx, scope, vql, os.Stdout)
		table.Render()

	case "jsonl":
		outputJSONL(ctx, scope, vql, os.Stdout)

	case "json":
		outputJSON(ctx, scope, vql, os.Stdout)
	}
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case csv_cmd.FullCommand():
			doCSV()

		default:
			return false
		}
		return true
	})
}
