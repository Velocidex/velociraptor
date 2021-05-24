// +build server_vql

package main

import (
	"bytes"
	"context"
	"log"
	"os"

	"github.com/Velocidex/ordereddict"
	errors "github.com/pkg/errors"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/tools"
)

var (
	report_command_archive = report_command.Command("archive", "Generate a report on an existing archive.")

	report_command_archive_report = report_command_archive.Flag(
		"artifact", "An artifact that contains a report to generate (default Reporting.Default).").
		Default("Reporting.Default").String()

	report_command_archive_file = report_command_archive.Arg(
		"archive", "A path to an archive file.").
		Required().String()

	report_command_archive_output = report_command_archive.Arg(
		"output", "A path to an output file to write on.").
		String()
)

func doReportArchive() {
	config_obj, err := makeDefaultConfigLoader().
		WithNullLoader().LoadAndValidate()
	kingpin.FatalIfError(err, "Unable to load config file")

	sm, err := startEssentialServices(config_obj)
	kingpin.FatalIfError(err, "Starting services.")
	defer sm.Close()

	builder := services.ScopeBuilder{
		Config:     config_obj,
		ACLManager: vql_subsystem.NullACLManager{},
		Logger:     log.New(&LogWriter{config_obj}, " ", 0),
		Env:        ordereddict.NewDict(),
	}
	manager, err := services.GetRepositoryManager()
	kingpin.FatalIfError(err, "GetRepositoryManager")

	scope := manager.BuildScopeFromScratch(builder)
	defer scope.Close()

	archive, err := reporting.NewArchiveReader(*report_command_archive_file)

	kingpin.FatalIfError(err, "Unable to open archive file")

	repository, err := getRepository(config_obj)
	kingpin.FatalIfError(err, "Unable to load artifacts")

	parts := []*ReportPart{}
	main := ""

	template := *report_command_archive_report
	html_template_string, err := getHTMLTemplate(config_obj, template,
		repository)
	kingpin.FatalIfError(err, "Unable to load report %v", template)

	for _, name := range archive.ListArtifacts() {
		scope := manager.BuildScopeFromScratch(builder)
		defer scope.Close()

		// Reports can query the container directly.
		scope.AppendPlugins(&tools.ArchiveSourcePlugin{
			Archive: archive})

		definition, pres := repository.Get(config_obj, name)
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

			res, err := reporting.GenerateClientReport(
				template_engine, "", "", nil)
			kingpin.FatalIfError(err, "Unable to render")

			content_writer.Write([]byte(res))
		}
		parts = append(parts, &ReportPart{
			Artifact: definition, HTML: content_writer.String()})
		main += content_writer.String()
	}

	// Reports can query the container directly.
	scope.AppendPlugins(&tools.ArchiveSourcePlugin{
		Archive: archive})

	template_engine, err := reporting.NewHTMLTemplateEngine(
		config_obj, context.Background(), scope,
		vql_subsystem.NullACLManager{}, repository,
		template, false /* sanitize_html */)
	kingpin.FatalIfError(err, "Unable to render")

	template_engine.SetEnv("main", main)
	template_engine.SetEnv("parts", parts)

	result, err := template_engine.RenderRaw(
		html_template_string, template_engine.Env.ToDict())
	kingpin.FatalIfError(err, "Unable to render")

	writer := os.Stdout
	if *report_command_archive_output != "" {
		writer, err = os.OpenFile(
			*report_command_archive_output,
			os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
		kingpin.FatalIfError(err, "Unable to open output file")
		defer writer.Close()
	}

	_, err = writer.Write([]byte(result))
	kingpin.FatalIfError(err, "Unable to write")
}

type ReportPart struct {
	Artifact *artifacts_proto.Artifact
	HTML     string
}

func getHTMLTemplate(
	config_obj *config_proto.Config,
	name string, repository services.Repository) (string, error) {
	template_artifact, ok := repository.Get(config_obj, name)
	if !ok || len(template_artifact.Reports) == 0 {
		return "", errors.New("Not found")
	}

	for _, report := range template_artifact.Reports {
		if report.Type == "html" {
			return report.Template, nil
		}
	}
	return "", errors.New("Not found")
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case report_command_archive.FullCommand():
			doReportArchive()

		default:
			return false
		}
		return true
	})
}
