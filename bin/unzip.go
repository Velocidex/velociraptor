package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/Velocidex/ordereddict"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/uploads"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	unzip_cmd        = app.Command("unzip", "Convert a CSV file to another format")
	unzip_cmd_filter = unzip_cmd.Flag("where", "A WHERE condition for the query").String()

	unzip_path = unzip_cmd.Flag("dump_dir", "Directory to dump output files.").
			Default(".").String()

	unzip_format = unzip_cmd.Flag("format", "Output format for csv output").
			Default("json").Enum("text", "json", "jsonl")
	unzip_cmd_list = unzip_cmd.Flag("list", "List files in the zip").Short('l').Bool()
	unzip_cmd_csv  = unzip_cmd.Flag("csv", "Parse CSV files and emit rows in default format").
			Short('C').Bool()

	unzip_cmd_file   = unzip_cmd.Arg("file", "Zip file to parse").Required().String()
	unzip_cmd_member = unzip_cmd.Arg("members", "Members glob to extract").Default("/**").String()
)

func doUnzip() {
	config_obj, err := DefaultConfigLoader.WithNullLoader().LoadAndValidate()
	kingpin.FatalIfError(err, "Load Config")

	filename, err := filepath.Abs(*unzip_cmd_file)
	kingpin.FatalIfError(err, "File does not exist")

	_, err = os.Stat(filename)
	if os.IsNotExist(err) {
		kingpin.FatalIfError(err, "File does not exist")
	}

	builder := artifacts.ScopeBuilder{
		ACLManager: vql_subsystem.NewRoleACLManager("administrator"),
		Logger:     log.New(&LogWriter{config_obj}, "Velociraptor: ", log.Lshortfile),
		Env: ordereddict.NewDict().
			Set("ZipPath", filename).
			Set("MemberGlob", *unzip_cmd_member),
	}

	var query string

	if *unzip_cmd_csv {
		query = `
       SELECT * FROM foreach(
         row={
           SELECT FullPath
           FROM glob(globs=url(scheme='file',
                           path=ZipPath,
                           fragment=MemberGlob).String,
                 accessor='zip')
           WHERE NOT IsDir AND Name =~ "\\.csv$"
         }, query={
           SELECT * FROM parse_csv(filename=FullPath, accessor='zip')
       })`
		if *unzip_cmd_filter != "" {
			query += " WHERE " + *unzip_cmd_filter
		}

	} else if *unzip_cmd_list {
		query = `
       SELECT url(parse=FullPath).Fragment AS Filename,
              Size
       FROM glob(globs=url(scheme='file',
                           path=ZipPath,
                           fragment=MemberGlob).String,
                 accessor='zip')
       WHERE NOT IsDir`

		if *unzip_cmd_filter != "" {
			query += " AND " + *unzip_cmd_filter
		}

	} else {
		builder.Uploader = &uploads.FileBasedUploader{
			UploadDir: *unzip_path,
		}

		query = `
       SELECT upload(
               file=FullPath, accessor='zip',
               name=url(parse=FullPath).Fragment) AS Extracted
       FROM glob(globs=url(scheme='file',
                           path=ZipPath,
                           fragment=MemberGlob).String,
                 accessor='zip')
       WHERE NOT IsDir`

		if *unzip_cmd_filter != "" {
			query += " AND " + *unzip_cmd_filter
		}
	}

	scope := builder.Build()
	defer scope.Close()

	vql, err := vfilter.Parse(query)
	kingpin.FatalIfError(err, "Unable to parse VQL Query")

	ctx := InstallSignalHandler(scope)

	scope.Log("Running query %v", query)

	switch *unzip_format {
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
		case unzip_cmd.FullCommand():
			doUnzip()

		default:
			return false
		}
		return true
	})
}
