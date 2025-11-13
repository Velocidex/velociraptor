package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/startup"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/vfilter"
)

var (
	unzip_cmd                 = app.Command("unzip", "Unzip a container file")
	unzip_cmd_report_password = unzip_cmd.Flag("report_password", "Log the X509 session password").Bool()
	unzip_cmd_filter          = unzip_cmd.Flag("where", "A WHERE condition for the query").String()

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
	logging.DisableLogging()

	server_config_obj, err := makeDefaultConfigLoader().
		WithNullLoader().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	config_obj := &config_proto.Config{}
	config_obj.Frontend = server_config_obj.Frontend

	config_obj.Services = services.GenericToolServices()
	sm, err := startup.StartToolServices(ctx, config_obj)
	defer sm.Close()

	if err != nil {
		return err
	}

	filename, err := filepath.Abs(*unzip_cmd_file)
	if err != nil {
		return err
	}

	_, err = os.Stat(filename)
	if os.IsNotExist(err) {
		return err
	}

	logger := &LogWriter{config_obj: sm.Config}
	builder := services.ScopeBuilder{
		Config:     sm.Config,
		ACLManager: acl_managers.NewRoleACLManager(sm.Config, "administrator"),
		Logger:     log.New(logger, "", 0),
		Env: ordereddict.NewDict().
			Set("ZipPath", filename).
			Set("DumpDir", *unzip_path).
			Set("MemberGlob", *unzip_cmd_member).
			Set(constants.REPORT_ZIP_PASSWORD, *unzip_cmd_report_password),
	}

	if *unzip_cmd_list {
		err = runUnzipList(builder)
	} else if *unzip_cmd_print {
		err = runUnzipPrint(builder)
	} else {
		err = runUnzipFiles(builder)
	}
	if err != nil {
		return err
	}

	return logger.Error
}

func runUnzipList(builder services.ScopeBuilder) error {
	query := `
       SELECT OSPath.Path AS Filename,
              Size
       FROM glob(globs=MemberGlob,
                 root=pathspec(DelegatePath=ZipPath),
                 accessor='collector')
       WHERE NOT IsDir`

	if *unzip_cmd_filter != "" {
		query += " AND " + *unzip_cmd_filter
	}

	return runQueryWithEnv(query, builder, *unzip_format)
}

func runUnzipFiles(builder services.ScopeBuilder) error {
	builder.Uploader = &uploads.FileBasedUploader{
		UploadDir: *unzip_path,
	}

	query := `
       SELECT upload(
               file=OSPath, accessor='collector',
               name=OSPath.Path) AS Extracted
       FROM glob(globs=MemberGlob,
                 root=pathspec(DelegatePath=ZipPath),
                 accessor='collector')
       WHERE NOT IsDir`

	if *unzip_cmd_filter != "" {
		query += " AND " + *unzip_cmd_filter
	}

	return runQueryWithEnv(query, builder, *unzip_format)
}

func runUnzipPrint(builder services.ScopeBuilder) error {
	query := `
       SELECT * FROM foreach(
       row={
          SELECT OSPath
          FROM glob(globs=MemberGlob,
                    root=pathspec(DelegatePath=ZipPath),
                    accessor='collector')
          WHERE NOT IsDir AND OSPath =~ '.json$'
       }, query={
           SELECT *
          FROM parse_jsonl(filename=OSPath, accessor='collector')
       })
    `
	return runQueryWithEnv(query, builder, *unzip_format)
}

func runQueryWithEnv(
	query string, builder services.ScopeBuilder, format string) error {
	manager, err := services.GetRepositoryManager(builder.Config)
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
		scope.Log("Running query %v", vfilter.FormatToString(scope, vql))

		switch format {
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
