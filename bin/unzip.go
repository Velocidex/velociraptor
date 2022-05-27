package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Velocidex/ordereddict"
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

func doUnzip() error {
	config_obj, err := makeDefaultConfigLoader().WithNullLoader().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	sm, err := startEssentialServices(config_obj)
	if err != nil {
		return fmt.Errorf("Starting services: %w", err)
	}
	defer sm.Close()

	filename, err := filepath.Abs(*unzip_cmd_file)
	if err != nil {
		return err
	}

	_, err = os.Stat(filename)
	if os.IsNotExist(err) {
		return err
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
		return runUnzipList(builder)
	} else if *unzip_cmd_print {
		return runUnzipPrint(builder)
	} else {
		return runUnzipFiles(builder)
	}
}

func runUnzipList(builder services.ScopeBuilder) error {
	query := `
       SELECT OSPath.Path AS Filename,
              Size
       FROM glob(globs=MemberGlob,
                 root=pathspec(DelegatePath=ZipPath),
                 accessor='zip')
       WHERE NOT IsDir`

	if *unzip_cmd_filter != "" {
		query += " AND " + *unzip_cmd_filter
	}

	return runQueryWithEnv(query, builder)
}

func runUnzipFiles(builder services.ScopeBuilder) error {
	builder.Uploader = &uploads.FileBasedUploader{
		UploadDir: *unzip_path,
	}

	query := `
       SELECT upload(
               file=OSPath, accessor='zip',
               name=OSPath.Path) AS Extracted
       FROM glob(globs=MemberGlob,
                 root=pathspec(DelegatePath=ZipPath),
                 accessor='zip')
       WHERE NOT IsDir`

	if *unzip_cmd_filter != "" {
		query += " AND " + *unzip_cmd_filter
	}

	return runQueryWithEnv(query, builder)
}

func runUnzipPrint(builder services.ScopeBuilder) error {
	query := `
       SELECT * FROM foreach(
       row={
          SELECT OSPath
          FROM glob(globs=MemberGlob,
                    root=pathspec(DelegatePath=ZipPath),
                    accessor='zip')
          WHERE NOT IsDir AND FullPath =~ '.json$'
       }, query={
          SELECT *
          FROM parse_jsonl(filename=OSPath, accessor='zip')
       })
    `
	return runQueryWithEnv(query, builder)
}

func getAllStats(query string, builder services.ScopeBuilder) (
	[]*ordereddict.Dict, error) {
	manager, err := services.GetRepositoryManager()
	if err != nil {
		return nil, err
	}

	scope := manager.BuildScope(builder)
	defer scope.Close()

	vql, err := vfilter.Parse(query)
	if err != nil {
		return nil, fmt.Errorf("Unable to parse VQL Query: %w", err)
	}

	ctx, cancel := InstallSignalHandler(nil, scope)
	defer cancel()

	result := []*ordereddict.Dict{}
	for row := range vql.Eval(ctx, scope) {
		d, ok := row.(*ordereddict.Dict)
		if ok {
			result = append(result, d)
		}
	}
	return result, nil
}

func runQueryWithEnv(
	query string, builder services.ScopeBuilder) error {
	manager, err := services.GetRepositoryManager()
	if err != nil {
		return err
	}

	scope := manager.BuildScope(builder)
	defer scope.Close()

	vqls, err := vfilter.MultiParse(query)
	if err != nil {
		return fmt.Errorf("Unable to parse VQL Query: %w", err)
	}

	ctx, cancel := InstallSignalHandler(nil, scope)
	defer cancel()

	for _, vql := range vqls {
		scope.Log("Running query %v", query)

		switch *unzip_format {
		case "text":
			table := reporting.EvalQueryToTable(ctx, scope, vql, os.Stdout)
			table.Render()

		case "jsonl":
			err := outputJSONL(ctx, scope, vql, os.Stdout)
			if err != nil {
				return err
			}
		case "json":
			err = outputJSON(ctx, scope, vql, os.Stdout)
			if err != nil {
				return err
			}

		case "csv":
			err = outputCSV(ctx, builder.Config, scope, vql, os.Stdout)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case unzip_cmd.FullCommand():
			FatalIfError(unzip_cmd, doUnzip)

		default:
			return false
		}
		return true
	})
}
