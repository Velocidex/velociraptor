package main

import (
	"log"
	"os"
	"strings"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vql_networking "www.velocidex.com/golang/velociraptor/vql/networking"
	vfilter "www.velocidex.com/golang/vfilter"
)

var (
	fs_command          = app.Command("fs", "Run filesystem commands.")
	fs_command_accessor = fs_command.Flag(
		"accessor", "The FS accessor to use").Default("file").Enum(
		"file", "ntfs", "reg")
	fs_command_verbose = fs_command.Flag(
		"verbose", "Show more verbose info").Short('v').
		Default("false").Bool()
	fs_command_format = fs_command.Flag("format", "Output format to use.").
				Default("text").Enum("text", "json")

	fs_command_ls      = fs_command.Command("ls", "List files")
	fs_command_ls_path = fs_command_ls.Arg(
		"path", "The path to list").Default("/").String()

	fs_command_cp      = fs_command.Command("cp", "Copy files to a directory.")
	fs_command_cp_path = fs_command_cp.Arg(
		"path", "The path to list").Default("/").String()
	fs_command_cp_outdir = fs_command_cp.Arg(
		"dumpdir", "The directory to store files at.").Default(".").
		ExistingDir()
)

func eval_query(query string, scope *vfilter.Scope) {
	vql, err := vfilter.Parse(query)
	if err != nil {
		kingpin.FatalIfError(err, "Unable to parse VQL Query")
	}

	switch *fs_command_format {
	case "text":
		table := evalQueryToTable(scope, vql)
		table.Render()
	case "json":
		outputJSON(scope, vql)
	}
}

func doLS(path string) {
	if len(path) > 0 && (path[len(path)-1] == '/' ||
		path[len(path)-1] == '\\') {
		path += "*"
	}

	env := vfilter.NewDict().
		Set("accessor", *fs_command_accessor).
		Set("path", path)

	scope := vql_subsystem.MakeScope().AppendVars(env)
	scope.Logger = log.New(os.Stderr, "velociraptor: ", log.Lshortfile)

	query := "SELECT Name, Size, Mode.String AS Mode, " +
		"timestamp(epoch=Mtime.Sec) as mtime, Data " +
		"FROM glob(globs=path, accessor=accessor) "
	if *fs_command_verbose {
		query = strings.Replace(query, "Name", "FullPath", 1)
	}
	// Special handling for ntfs.
	if !*fs_command_verbose && *fs_command_accessor == "ntfs" {
		query += " WHERE Sys.name_type != 'DOS' "
	}

	eval_query(query, scope)
}

func doCp(path string, dump_dir string) {
	if len(path) > 0 && (path[len(path)-1] == '/' ||
		path[len(path)-1] == '\\') {
		path += "*"
	}

	env := vfilter.NewDict().
		Set("accessor", *fs_command_accessor).
		Set("path", path).
		Set("$uploader", &vql_networking.FileBasedUploader{dump_dir})

	scope := vql_subsystem.MakeScope().AppendVars(env)
	scope.Logger = log.New(os.Stderr, "velociraptor: ", log.Lshortfile)

	eval_query(`SELECT * from foreach(
  row={
    SELECT Name, Size, Mode.String AS Mode,
       timestamp(epoch=Mtime.Sec) as mtime, Data, FullPath
    FROM glob(globs=path, accessor=accessor)
    WHERE Sys.name_type != 'DOS'
  }, query={
     SELECT Name, Size, Mode, mtime, Data,
     upload(file=FullPath, accessor=accessor) AS Upload
     FROM scope()
  })`, scope)
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case "fs ls":
			doLS(*fs_command_ls_path)
		case "fs cp":
			doCp(*fs_command_cp_path, *fs_command_cp_outdir)

		default:
			return false
		}
		return true
	})
}
