package main

import (
	"log"
	"os"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

var (
	fs_command          = app.Command("fs", "Run filesystem commands.")
	fs_command_accessor = fs_command.Flag(
		"accessor", "The FS accessor to use").Default("file").Enum(
		"file", "ntfs", "reg")

	fs_command_command      = fs_command.Command("ls", "List files")
	fs_command_command_path = fs_command_command.Arg(
		"path", "The path to list").Default("/").String()

	fs_command_format = fs_command.Flag("format", "Output format to use.").
				Default("text").Enum("text", "json")
)

func doLS() {
	path := *fs_command_command_path
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
		"from glob(globs=path, accessor=accessor)"

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

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case "fs ls":
			doLS()
		default:
			return false
		}
		return true
	})
}
