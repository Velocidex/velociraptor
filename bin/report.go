// +build server_vql

package main

import (
	"log"
	"os"

	"github.com/Velocidex/ordereddict"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/server/downloads"
)

var (
	report_command = app.Command("report", "Generate a report.")

	report_command_flow = report_command.Command("flow", "Report on a collection")

	report_command_flow_report = report_command_flow.Flag(
		"artifact", "An artifact that contains a report to generate (default Reporting.Default).").
		Default("Reporting.Default").String()

	report_command_flow_client = report_command_flow.Arg(
		"client_id", "The client id to generate the report for.").
		Required().String()

	report_command_flow_flow_id = report_command_flow.Arg(
		"flow_id", "The flow id to generate the report for.").
		Required().String()

	report_command_flow_output = report_command_flow.Arg(
		"output", "A path to an output file to write on.").
		String()
)

func doFlowReport() {
	config_obj, err := APIConfigLoader.WithNullLoader().LoadAndValidate()
	kingpin.FatalIfError(err, "Load Config ")

	sm, err := startEssentialServices(config_obj)
	kingpin.FatalIfError(err, "Starting services.")
	defer sm.Close()

	builder := services.ScopeBuilder{
		Config: config_obj,
		Logger: log.New(&LogWriter{config_obj}, "", 0),
		Env: ordereddict.NewDict().
			Set("ClientId", *report_command_flow_client).
			Set("FlowId", *report_command_flow_flow_id),
		ACLManager: vql_subsystem.NewRoleACLManager("administrator"),
	}

	scope := services.GetRepositoryManager().BuildScope(builder)
	defer scope.Close()

	writer := os.Stdout
	if *report_command_flow_output != "" {
		writer, err = os.OpenFile(
			*report_command_flow_output,
			os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
		kingpin.FatalIfError(err, "Unable to open output file")
		defer writer.Close()
	}

	repository, err := getRepository(config_obj)
	kingpin.FatalIfError(err, "Repository")

	err = downloads.WriteFlowReport(config_obj, scope, repository,
		writer, *report_command_flow_flow_id,
		*report_command_flow_client, *report_command_flow_report)
	kingpin.FatalIfError(err, "Generating report")
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case report_command_flow.FullCommand():
			doFlowReport()

		default:
			return false
		}
		return true
	})
}
