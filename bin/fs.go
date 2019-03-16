/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package main

import (
	"log"
	"os"
	"strings"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/velociraptor/reporting"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vql_networking "www.velocidex.com/golang/velociraptor/vql/networking"
	vfilter "www.velocidex.com/golang/vfilter"
)

var (
	fs_command          = app.Command("fs", "Run filesystem commands.")
	fs_command_accessor = fs_command.Flag(
		"accessor", "The FS accessor to use").Default("file").Enum(
		"file", "ntfs", "reg", "zip", "raw_reg")
	fs_command_verbose = fs_command.Flag(
		"details", "Show more verbose info").Short('d').
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

	ctx := InstallSignalHandler(scope)

	switch *fs_command_format {
	case "text":
		table := reporting.EvalQueryToTable(ctx, scope, vql, os.Stdout)
		table.Render()
	case "json":
		outputJSON(ctx, scope, vql, os.Stdout)
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
	defer scope.Close()

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
		Set("$uploader", &vql_networking.FileBasedUploader{
			UploadDir: dump_dir,
		}).
		Set(vql_subsystem.CACHE_VAR, vql_subsystem.NewScopeCache())

	scope := vql_subsystem.MakeScope().AppendVars(env)
	defer scope.Close()

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
