// +build server_vql

package main

import (
	"bytes"
	"context"
	"log"
	"os"

	"github.com/Velocidex/ordereddict"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/flows"
	"www.velocidex.com/golang/velociraptor/reporting"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
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

func doHTMLReport() {
	config_obj, err := DefaultConfigLoader.WithRequiredFrontend().LoadAndValidate()
	kingpin.FatalIfError(err, "Unable to load config file")

	repository, err := getRepository(config_obj)
	kingpin.FatalIfError(err, "Unable to load artifacts")

	flow_details, err := flows.GetFlowDetails(config_obj, *report_command_flow_client,
		*report_command_flow_flow_id)
	kingpin.FatalIfError(err, "Unable to load flow")

	if flow_details.Context == nil {
		kingpin.Fatalf("Unable to open flow %v", *report_command_flow_flow_id)
	}

	env := ordereddict.NewDict()
	builder := artifacts.ScopeBuilder{
		Config:     config_obj,
		ACLManager: vql_subsystem.NullACLManager{},
		Logger:     log.New(&LogWriter{config_obj}, " ", 0),
		Env:        env,
	}
	scope := builder.BuildFromScratch()
	defer scope.Close()

	parts := []*ReportPart{}
	main := ""

	template := *report_command_flow_report
	html_template_string, err := getHTMLTemplate(
		template, repository)
	kingpin.FatalIfError(err, "Unable to load report %v", template)

	for _, name := range flow_details.Context.Request.Artifacts {
		definition, pres := repository.Get(name)
		if !pres {
			scope.Log("Artifact %v not found %v\n", name, err)
			continue
		}

		content_writer := &bytes.Buffer{}

		scope.Log("Rendering artifact %v\n", definition.Name)
		for _, report := range definition.Reports {
			if report.Type != "client" {
				continue
			}

			// Do not sanitize_html since we are writing a
			// stand along HTML file - artifacts may
			// generate arbitrary HTML.
			template_engine, err := reporting.NewHTMLTemplateEngine(
				config_obj, context.Background(), scope,
				vql_subsystem.NullACLManager{}, repository,
				definition.Name, false /* sanitize_html */)
			kingpin.FatalIfError(err, "Unable to render")

			for _, param := range report.Parameters {
				template_engine.SetEnv(param.Name, param.Default)
			}

			template_engine.SetEnv("ClientId", *report_command_flow_client)
			template_engine.SetEnv("FlowId", *report_command_flow_flow_id)

			res, err := reporting.GenerateClientReport(
				template_engine, "", "", nil)
			kingpin.FatalIfError(err, "Unable to render")

			content_writer.Write([]byte(res))
		}
		parts = append(parts, &ReportPart{
			Artifact: definition, HTML: content_writer.String()})
		main += content_writer.String()
	}

	template_engine, err := reporting.NewHTMLTemplateEngine(
		config_obj, context.Background(), scope,
		vql_subsystem.NullACLManager{}, repository,
		template, false /* sanitize_html */)
	kingpin.FatalIfError(err, "Unable to render")

	template_engine.SetEnv("main", main)
	template_engine.SetEnv("parts", parts)
	template_engine.SetEnv("ClientId", *report_command_flow_client)
	template_engine.SetEnv("FlowId", *report_command_flow_flow_id)

	result, err := template_engine.RenderRaw(
		html_template_string, template_engine.Env.ToDict())
	kingpin.FatalIfError(err, "Unable to render")

	writer := os.Stdout
	if *report_command_flow_output != "" {
		writer, err = os.OpenFile(
			*report_command_flow_output,
			os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
		kingpin.FatalIfError(err, "Unable to open output file")
		defer writer.Close()
	}

	writer.Write([]byte(result))
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
