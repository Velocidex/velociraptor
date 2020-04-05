package main

import (
	"os"

	"github.com/Velocidex/ordereddict"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
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
	env := ordereddict.NewDict().
		Set(vql_subsystem.ACL_MANAGER_VAR,
			vql_subsystem.NewRoleACLManager("administrator")).
		Set("Files", *csv_cmd_files)

	scope := vql_subsystem.MakeScope().AppendVars(env)
	defer scope.Close()

	AddLogger(scope, get_config_or_default())
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
