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
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Velocidex/ordereddict"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/uploads"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

var (
	accessor_reg = regexp.MustCompile(
		"^(file|ntfs|reg|registry|zip|raw_reg|lazy_ntfs|file_links|fs)://(.+)$")

	fs_command          = app.Command("fs", "Run filesystem commands.")
	fs_command_accessor = fs_command.Flag(
		"accessor", "The FS accessor to use").Default("file").String()
	fs_command_verbose = fs_command.Flag(
		"details", "Show more verbose info").Short('l').
		Default("false").Bool()
	fs_command_format = fs_command.Flag("format", "Output format to use  (text,json,jsonl,csv).").
				Default("jsonl").Enum("text", "json", "jsonl", "csv")

	fs_command_ls      = fs_command.Command("ls", "List files")
	fs_command_ls_path = fs_command_ls.Arg(
		"path", "The path or glob to list").Default("/").String()

	fs_command_cp      = fs_command.Command("cp", "Copy files to a directory.")
	fs_command_cp_path = fs_command_cp.Arg(
		"path", "The path or glob to list").Required().String()
	fs_command_cp_outdir = fs_command_cp.Arg(
		"dumpdir", "The directory to store files at.").Required().
		String()

	fs_command_cat      = fs_command.Command("cat", "Dump a file to the terminal")
	fs_command_cat_path = fs_command_cat.Arg(
		"path", "The path to cat").Required().String()

	fs_command_rm      = fs_command.Command("rm", "Remove file (only filestore supported)")
	fs_command_rm_path = fs_command_rm.Arg(
		"path", "The path or glob to remove").Required().String()
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

	case "jsonl":
		outputJSONL(ctx, scope, vql, os.Stdout)

	case "json":
		outputJSON(ctx, scope, vql, os.Stdout)
	}
}

func doLS(path, accessor string) {
	initFilestoreAccessor()

	matches := accessor_reg.FindStringSubmatch(path)
	if matches != nil {
		accessor = matches[1]
		path = matches[2]
	}

	if len(path) > 0 && (path[len(path)-1] == '/' ||
		path[len(path)-1] == '\\') {
		path += "*"
	}

	env := ordereddict.NewDict().
		Set(vql_subsystem.ACL_MANAGER_VAR,
			vql_subsystem.NewRoleACLManager("administrator")).
		Set("accessor", accessor).
		Set("path", path)

	scope := vql_subsystem.MakeScope().AppendVars(env)
	defer scope.Close()

	AddLogger(scope, get_config_or_default())

	query := "SELECT Name, Size, Mode.String AS Mode, Mtime, Data " +
		"FROM glob(globs=path, accessor=accessor) "
	if *fs_command_verbose {
		query = strings.Replace(query, "Name", "FullPath", 1)
	}

	// Special handling for ntfs.
	if !*fs_command_verbose && accessor == "ntfs" {
		query += " WHERE Sys.name_type != 'DOS' "
	}

	eval_query(query, scope)
}

func doRM(path, accessor string) {
	initFilestoreAccessor()

	matches := accessor_reg.FindStringSubmatch(path)
	if matches != nil {
		accessor = matches[1]
		path = matches[2]
	}

	if len(path) > 0 && (path[len(path)-1] == '/' ||
		path[len(path)-1] == '\\') {
		path += "*"
	}

	if accessor != "fs" {
		kingpin.Fatalf("Only fs:// URLs support removal")
	}

	config_obj := get_config_or_default()
	scope := artifacts.ScopeBuilder{
		Config:     config_obj,
		ACLManager: vql_subsystem.NewRoleACLManager("administrator"),
		Env: ordereddict.NewDict().
			Set("accessor", accessor).
			Set("path", path),
	}.Build()
	defer scope.Close()

	AddLogger(scope, get_config_or_default())

	query := "SELECT FullPath, Size, Mode.String AS Mode, Mtime, " +
		"file_store_delete(path=FullPath) AS Deletion " +
		"FROM glob(globs=path, accessor=accessor) "

	eval_query(query, scope)
}

func doCp(path, accessor string, dump_dir string) {
	initFilestoreAccessor()
	config_obj := get_config_or_default()

	matches := accessor_reg.FindStringSubmatch(path)
	if matches != nil {
		accessor = matches[1]
		path = matches[2]
	}

	if len(path) > 0 && (path[len(path)-1] == '/' ||
		path[len(path)-1] == '\\') {
		path += "*"
	}

	if accessor == "file" {
		path, _ = filepath.Abs(path)
	}

	output_accessor := ""
	output_path := dump_dir

	matches = accessor_reg.FindStringSubmatch(dump_dir)
	if matches != nil {
		output_accessor = matches[1]
		output_path = matches[2]
	}

	builder := artifacts.ScopeBuilder{
		Config: config_obj,
		Env: ordereddict.NewDict().
			Set("accessor", accessor).
			Set("path", path),
		ACLManager: vql_subsystem.NewRoleACLManager("administrator"),
	}

	switch output_accessor {
	case "", "file":
		builder.Uploader = &uploads.FileBasedUploader{
			UploadDir: output_path,
		}

	case "fs":
		builder.Uploader = api.NewFileStoreUploader(
			config_obj,
			file_store.GetFileStore(config_obj),
			output_path)

	default:
		kingpin.Fatalf("Can not write to accessor %v\n", output_accessor)
	}

	scope := builder.Build()
	defer scope.Close()

	AddLogger(scope, get_config_or_default())

	scope.Log("Copy from %v (%v) to %v (%v)",
		path, accessor, output_path, output_accessor)

	eval_query(`
SELECT * from foreach(
  row={
    SELECT Name, Size, Mode.String AS Mode,
       Mtime, Data, FullPath
    FROM glob(globs=path, accessor=accessor)
  }, query={
     SELECT Name, Size, Mode, Mtime, Data,
     upload(file=FullPath, accessor=accessor, name=Name) AS Upload
     FROM scope()
  })`, scope)
}

func initFilestoreAccessor() {
	config_obj, err := get_server_config(*config_path)
	if err != nil {
		return
	}

	accessor, err := file_store.GetFileStoreFileSystemAccessor(config_obj)
	kingpin.FatalIfError(err, "GetFileStoreFileSystemAccessor")
	glob.Register("fs", accessor)
}

func doCat(path, accessor_name string) {
	initFilestoreAccessor()
	matches := accessor_reg.FindStringSubmatch(path)
	if matches != nil {
		accessor_name = matches[1]
		path = matches[2]
	}

	scope := vql_subsystem.MakeScope()
	accessor, err := glob.GetAccessor(accessor_name, scope)
	kingpin.FatalIfError(err, "GetAccessor")

	fd, err := accessor.Open(path)
	kingpin.FatalIfError(err, "ReadFile")

	io.Copy(os.Stdout, fd)
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case fs_command_ls.FullCommand():
			doLS(*fs_command_ls_path, *fs_command_accessor)

		case fs_command_rm.FullCommand():
			doRM(*fs_command_rm_path, *fs_command_accessor)

		case fs_command_cp.FullCommand():
			doCp(*fs_command_cp_path, *fs_command_accessor, *fs_command_cp_outdir)

		case fs_command_cat.FullCommand():
			doCat(*fs_command_cat_path, *fs_command_accessor)

		default:
			return false
		}
		return true
	})
}
