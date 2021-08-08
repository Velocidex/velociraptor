package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/Velocidex/ordereddict"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/uploads"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	unzip_cmd        = app.Command("unzip", "Unzip a container file")
	unzip_cmd_filter = unzip_cmd.Flag("where", "A WHERE condition for the query").String()

	unzip_path = unzip_cmd.Flag("dump_dir", "Directory to dump output files.").
			Default(".").String()

	unzip_format = unzip_cmd.Flag("format", "Output format for csv output").
			Default("json").Enum("text", "json", "csv", "jsonl")

	unzip_cmd_list = unzip_cmd.Flag("list", "List files in the zip").Short('l').Bool()

	unzip_cmd_print = unzip_cmd.Flag("print", "Dump out the files in the zip").Short('p').Bool()

	unzip_cmd_file = unzip_cmd.Arg("file", "Zip file to parse").Required().String()

	unzip_cmd_member = unzip_cmd.Arg("members", "Members glob to extract").Default("/**").String()
)

func doUnzip() {
	config_obj, err := makeDefaultConfigLoader().WithNullLoader().LoadAndValidate()
	kingpin.FatalIfError(err, "Load Config")

	sm, err := startEssentialServices(config_obj)
	kingpin.FatalIfError(err, "Starting services.")
	defer sm.Close()

	filename, err := filepath.Abs(*unzip_cmd_file)
	kingpin.FatalIfError(err, "File does not exist")

	_, err = os.Stat(filename)
	if os.IsNotExist(err) {
		kingpin.FatalIfError(err, "File does not exist")
	}

	builder := services.ScopeBuilder{
		Config:     config_obj,
		ACLManager: vql_subsystem.NewRoleACLManager("administrator"),
		Logger:     log.New(&LogWriter{config_obj}, "Velociraptor: ", 0),
		Env: ordereddict.NewDict().
			Set("ZipPath", filename).
			Set("DumpDir", *unzip_path).
			Set("MemberGlob", *unzip_cmd_member),
	}

	if *unzip_cmd_list {
		runUnzipList(builder)
	} else if *unzip_cmd_print {
		runUnzipPrint(builder)
	} else {
		runUnzipFiles(builder)
	}
}

func runUnzipList(builder services.ScopeBuilder) {
	query := `
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

	runQueryWithEnv(query, builder)
}

func runUnzipFiles(builder services.ScopeBuilder) {
	builder.Uploader = &uploads.FileBasedUploader{
		UploadDir: *unzip_path,
	}

	query := `
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

	runQueryWithEnv(query, builder)
}

func runUnzipPrint(builder services.ScopeBuilder) {
	query := `
       SELECT * FROM foreach(
       row={
          SELECT FullPath
          FROM glob(globs=url(scheme='file',
                              path=ZipPath,
                              fragment=MemberGlob).String,
                    accessor='zip')
          WHERE NOT IsDir AND FullPath =~ '.json$'
       }, query={
          SELECT *
          FROM parse_jsonl(filename=FullPath, accessor='zip')
       })
    `
	runQueryWithEnv(query, builder)
}

func getAllStats(query string, builder services.ScopeBuilder) []*ordereddict.Dict {
	manager, err := services.GetRepositoryManager()
	kingpin.FatalIfError(err, "GetRepositoryManager")

	scope := manager.BuildScope(builder)
	defer scope.Close()

	vql, err := vfilter.Parse(query)
	kingpin.FatalIfError(err, "Unable to parse VQL Query")

	ctx := InstallSignalHandler(scope)

	result := []*ordereddict.Dict{}
	for row := range vql.Eval(ctx, scope) {
		d, ok := row.(*ordereddict.Dict)
		if ok {
			result = append(result, d)
		}
	}
	return result
}

func runQueryWithEnv(query string, builder services.ScopeBuilder) {
	manager, err := services.GetRepositoryManager()
	kingpin.FatalIfError(err, "GetRepositoryManager")

	scope := manager.BuildScope(builder)
	defer scope.Close()

	vqls, err := vfilter.MultiParse(query)
	kingpin.FatalIfError(err, "Unable to parse VQL Query")

	ctx := InstallSignalHandler(scope)

	for _, vql := range vqls {
		scope.Log("Running query %v", query)

		switch *unzip_format {
		case "text":
			table := reporting.EvalQueryToTable(ctx, scope, vql, os.Stdout)
			table.Render()

		case "jsonl":
			outputJSONL(ctx, scope, vql, os.Stdout)

		case "json":
			outputJSON(ctx, scope, vql, os.Stdout)

		case "csv":
			outputCSV(ctx, scope, vql, os.Stdout)
		}
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
