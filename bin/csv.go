package main

import (
	"log"
	"os"

	"github.com/Velocidex/ordereddict"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/startup"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
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
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().
		WithNullLoader().LoadAndValidate()
	if err != nil {
		return err
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	config_obj.Services = services.GenericToolServices()
	sm, err := startup.StartToolServices(ctx, config_obj)
	defer sm.Close()

	if err != nil {
		return err
	}

	logger := &LogWriter{config_obj: config_obj}
	builder := services.ScopeBuilder{
		Config:     config_obj,
		ACLManager: acl_managers.NullACLManager{},
		Logger:     log.New(logger, "", 0),
		Env: ordereddict.NewDict().
			Set(vql_subsystem.ACL_MANAGER_VAR,
				acl_managers.NewRoleACLManager(config_obj, "administrator")).
			Set("Files", *csv_cmd_files),
	}

	manager, err := services.GetRepositoryManager(config_obj)
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

	switch *csv_format {
	case "text":
		table := reporting.EvalQueryToTable(ctx, scope, vql, os.Stdout)
		table.Render()

	case "jsonl":
		err = outputJSONL(ctx, scope, vql, os.Stdout)
		if err != nil {
			return err
		}

	case "json":
		err = outputJSON(ctx, scope, vql, os.Stdout)
		if err != nil {
			return err
		}

	}
	return logger.Error
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
