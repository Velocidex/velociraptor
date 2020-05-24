package main

import (
	"bytes"
	"context"
	"fmt"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/velociraptor/reporting"
)

var (
	notebook_command = report_command.Command(
		"notebook", "Export notebook as HTML.")

	notebook_command_notebook_id = notebook_command.Arg(
		"id", "Notebook ID to export").Required().String()
)

func doExportNotebook() {
	config_obj, err := DefaultConfigLoader.WithRequiredFrontend().LoadAndValidate()
	kingpin.FatalIfError(err, "Unable to load config file")

	ctx := context.Background()
	writer := &bytes.Buffer{}
	err = reporting.ExportNotebookToHTML(
		ctx, config_obj, *notebook_command_notebook_id, writer)
	kingpin.FatalIfError(err, "Generating report")

	fmt.Println(writer.String())
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case notebook_command.FullCommand():
			doExportNotebook()

		default:
			return false
		}
		return true
	})
}
