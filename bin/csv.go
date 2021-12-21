package main

import (
	"log"
	"os"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
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

func doCSV() error {
	config_obj, err := makeDefaultConfigLoader().
		WithNullLoader().LoadAndValidate()
	if err != nil {
		return err
	}

	sm, err := startEssentialServices(config_obj)
	if err != nil {
		return err
	}
	defer sm.Close()

	builder := services.ScopeBuilder{
		Config:     config_obj,
		ACLManager: vql_subsystem.NullACLManager{},
		Logger:     log.New(os.Stderr, "velociraptor: ", 0),
		Env: ordereddict.NewDict().
			Set(vql_subsystem.ACL_MANAGER_VAR,
				vql_subsystem.NewRoleACLManager("administrator")).
			Set("Files", *csv_cmd_files),
	}

	manager, err := services.GetRepositoryManager()
	if err != nil {
		return err
	}

	scope := manager.BuildScope(builder)
	defer scope.Close()

	query := "SELECT * FROM parse_csv(filename=Files)"

	if *csv_cmd_filter != "" {
		query += " WHERE " + *csv_cmd_filter
	}

	vql, err := vfilter.Parse(query)
	if err != nil {
		return err
	}

	ctx := InstallSignalHandler(scope)

	switch *csv_format {
	case "text":
		table := reporting.EvalQueryToTable(ctx, scope, vql, os.Stdout)
		table.Render()

	case "jsonl":
		return outputJSONL(ctx, scope, vql, os.Stdout)

	case "json":
		return outputJSON(ctx, scope, vql, os.Stdout)
	}
	return nil
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case csv_cmd.FullCommand():
			FatalIfError(csv_cmd, doCSV)

		default:
			return false
		}
		return true
	})
}
