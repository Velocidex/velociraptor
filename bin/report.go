package main

import (
	"context"
	"fmt"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/velociraptor/flows"
	"www.velocidex.com/golang/velociraptor/reporting"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

var (
	report_command = app.Command("report", "Generate a report.")

	report_command_flow = report_command.Command("flow", "Report on a collection")

	report_command_flow_client = report_command_flow.Arg(
		"client_id", "The client id to generate the report for.").
		Required().String()

	report_command_flow_flow_id = report_command_flow.Arg(
		"flow_id", "The flow id to generate the report for.").
		Required().String()
)

func doHTMLReport() {
	config_obj, err := DefaultConfigLoader.WithRequiredFrontend().LoadAndValidate()
	kingpin.FatalIfError(err, "Unable to load config file")

	repository, err := getRepository(config_obj)
	kingpin.FatalIfError(err, "Unable to load artifacts")

	result, err := flows.GetFlowDetails(config_obj, *report_command_flow_client,
		*report_command_flow_flow_id)
	kingpin.FatalIfError(err, "Unable to load flow")

	if result.Context == nil {
		kingpin.Fatalf("Unable to open flow %v", *report_command_flow_flow_id)
	}

	fmt.Println(reporting.HtmlPreable)
	defer fmt.Println(reporting.HtmlPostscript)

	for _, artifact_name := range result.Context.Request.Artifacts {
		template_engine, err := reporting.NewHTMLTemplateEngine(
			config_obj, context.Background(), nil, /* default scope */
			vql_subsystem.NullACLManager{}, repository, artifact_name)
		kingpin.FatalIfError(err, "Generating report")

		template_engine.SetEnv("ClientId", *report_command_flow_client)
		template_engine.SetEnv("FlowId", *report_command_flow_flow_id)

		for k, v := range *env_map {
			template_engine.SetEnv(k, v)
		}

		res, err := reporting.GenerateClientReport(
			template_engine,
			*report_command_flow_client, *report_command_flow_flow_id, nil)
		kingpin.FatalIfError(err, "Generating report")
		fmt.Println(res)
	}
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case report_command_flow.FullCommand():
			doHTMLReport()

		default:
			return false
		}
		return true
	})
}
