/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

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
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Velocidex/ordereddict"
	kingpin "github.com/alecthomas/kingpin/v2"
	"www.velocidex.com/golang/velociraptor/accessors"
	file_store_accessor "www.velocidex.com/golang/velociraptor/accessors/file_store"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/file_store/uploader"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/startup"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
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

	fs_command_zcat = fs_command.Command(
		"zcat", "Dump a compressed filestore file")
	fs_command_zcat_chunk_path = fs_command_zcat.Arg(
		"chunk_path", "The path to the .chunk index file").Required().File()
	fs_command_zcat_file_path = fs_command_zcat.Arg(
		"file_path", "The path to the compressed file to dump").Required().File()

	fs_command_rm      = fs_command.Command("rm", "Remove file (only filestore supported)")
	fs_command_rm_path = fs_command_rm.Arg(
		"path", "The path or glob to remove").Required().String()
)

func eval_query(
	ctx context.Context,
	config_obj *config_proto.Config,
	format, query string, scope vfilter.Scope,
	env *ordereddict.Dict) error {
	if config_obj.ApiConfig != nil && config_obj.ApiConfig.Name != "" {
		logging.GetLogger(config_obj, &logging.ToolComponent).
			Info("API Client configuration loaded - will make gRPC connection.")
		return doRemoteQuery(config_obj, format, config_obj.OrgId,
			[]string{query}, env)
	}

	return eval_local_query(ctx, config_obj, *fs_command_format, query, scope)
}

func eval_local_query(
	ctx context.Context,
	config_obj *config_proto.Config, format string,
	query string, scope vfilter.Scope) error {

	vqls, err := vfilter.MultiParse(query)
	if err != nil {
		return fmt.Errorf("Unable to parse VQL Query: %w", err)
	}

	for _, vql := range vqls {
		switch format {
		case "text":
			table := reporting.EvalQueryToTable(ctx, scope, vql, os.Stdout)
			table.Render()

		case "csv":
			return outputCSV(ctx, config_obj, scope, vql, os.Stdout)

		case "jsonl":
			return outputJSONL(ctx, scope, vql, os.Stdout)

		case "json":
			return outputJSON(ctx, scope, vql, os.Stdout)
		}
	}
	return nil
}

func doLS(path, accessor string) error {
	logging.DisableLogging()

	config_obj, err := APIConfigLoader.WithNullLoader().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	config_obj.Services = services.GenericToolServices()
	sm, err := startup.StartToolServices(ctx, config_obj)
	defer sm.Close()

	if err != nil {
		return err
	}

	matches := accessor_reg.FindStringSubmatch(path)
	if matches != nil {
		accessor = matches[1]
		path = matches[2]
	}

	if len(path) > 0 && (path[len(path)-1] == '/' ||
		path[len(path)-1] == '\\') {
		path += "*"
	}

	logger := &LogWriter{config_obj: config_obj}
	builder := services.ScopeBuilder{
		Config:     config_obj,
		ACLManager: acl_managers.NullACLManager{},
		Logger:     log.New(logger, "", 0),
		Env: ordereddict.NewDict().
			Set(vql_subsystem.ACL_MANAGER_VAR,
				acl_managers.NewRoleACLManager(config_obj, "administrator")).
			Set("accessor", accessor).
			Set("path", path),
	}

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return err
	}
	scope := manager.BuildScope(builder)
	defer scope.Close()

	query := "SELECT Name, Size, Mode.String AS Mode, Mtime, Data " +
		"FROM glob(globs=path, accessor=accessor) "
	if *fs_command_verbose {
		query = "SELECT OSPath, Size, Mode.String AS Mode, Mtime, Data " +
			"FROM glob(globs=path, accessor=accessor) "
	}

	// Special handling for ntfs.
	if !*fs_command_verbose && accessor == "ntfs" {
		query += " WHERE Sys.name_type != 'DOS' "
	}

	err = eval_query(sm.Ctx, config_obj,
		*fs_command_format, query, scope, builder.Env)
	if err != nil {
		return err
	}

	return logger.Error
}

func doRM(path, accessor string) error {
	logging.DisableLogging()

	config_obj, err := APIConfigLoader.WithNullLoader().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	sm, err := startup.StartToolServices(ctx, config_obj)
	defer sm.Close()

	if err != nil {
		return err
	}

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
		return fmt.Errorf("Only fs:// URLs support removal")
	}

	logger := &LogWriter{config_obj: config_obj}
	builder := services.ScopeBuilder{
		Config:     config_obj,
		ACLManager: acl_managers.NewRoleACLManager(config_obj, "administrator"),
		Logger:     log.New(logger, "", 0),
		Env: ordereddict.NewDict().
			Set("accessor", accessor).
			Set("path", path),
	}
	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return err
	}
	scope := manager.BuildScope(builder)
	defer scope.Close()

	query := "SELECT OSPath, Size, Mode.String AS Mode, Mtime, " +
		"file_store_delete(path=OSPath) AS Deletion " +
		"FROM glob(globs=path, accessor=accessor) "

	err = eval_query(sm.Ctx,
		config_obj, *fs_command_format, query, scope, builder.Env)
	if err != nil {
		return err
	}

	return logger.Error
}

func doCp(path, accessor string, dump_dir string) error {
	logging.DisableLogging()

	config_obj, err := APIConfigLoader.
		WithNullLoader().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	sm, err := startup.StartToolServices(ctx, config_obj)
	defer sm.Close()

	if err != nil {
		return err
	}

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

	logger := &LogWriter{config_obj: config_obj}
	builder := services.ScopeBuilder{
		Config: config_obj,
		Logger: log.New(logger, "", 0),
		Env: ordereddict.NewDict().
			Set("accessor", accessor).
			Set("path", path),
		ACLManager: acl_managers.NewRoleACLManager(config_obj, "administrator"),
	}

	switch output_accessor {
	case "", "file":
		builder.Uploader = &uploads.FileBasedUploader{
			UploadDir: output_path,
		}

	case "fs":
		output_path_spec := path_specs.NewSafeFilestorePath(output_path)
		builder.Uploader = uploader.NewFileStoreUploader(
			config_obj,
			file_store.GetFileStore(config_obj),
			output_path_spec)

	default:
		return fmt.Errorf("Can not write to accessor %v\n", output_accessor)
	}

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return err
	}
	scope := manager.BuildScope(builder)
	defer scope.Close()

	scope.Log("Copy from %v (%v) to %v (%v)",
		path, accessor, output_path, output_accessor)

	err = eval_query(sm.Ctx, config_obj, *fs_command_format, `
SELECT * from foreach(
  row={
    SELECT Name, Size, Mode.String AS Mode,
       Mtime, Data, OSPath
    FROM glob(globs=path, accessor=accessor)
  }, query={
     SELECT Name, Size, Mode, Mtime, Data,
     upload(file=OSPath, accessor=accessor, name=Name) AS Upload
     FROM scope()
  })`, scope, builder.Env)
	if err != nil {
		return err
	}

	return logger.Error
}

func doCat(path, accessor_name string) error {
	logging.DisableLogging()

	config_obj, err := APIConfigLoader.
		WithNullLoader().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	matches := accessor_reg.FindStringSubmatch(path)
	if matches != nil {
		accessor_name = matches[1]
		path = matches[2]
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	config_obj.Services = services.GenericToolServices()
	sm, err := startup.StartToolServices(ctx, config_obj)
	defer sm.Close()

	if err != nil {
		return err
	}

	logger := &LogWriter{config_obj: config_obj}
	builder := services.ScopeBuilder{
		Config:     config_obj,
		ACLManager: acl_managers.NullACLManager{},
		Logger:     log.New(logger, "", 0),
	}

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return err
	}
	scope := manager.BuildScope(builder)
	defer scope.Close()

	accessor, err := accessors.GetAccessor(accessor_name, scope)
	if err != nil {
		return err
	}

	fd, err := accessor.Open(path)
	if err != nil {
		return err
	}

	_, err = io.Copy(os.Stdout, fd)
	return err
}

func doZCat(chunk_fd, file_fd *os.File) error {
	defer chunk_fd.Close()
	defer file_fd.Close()

	if !strings.HasSuffix(chunk_fd.Name(), ".chunk") {
		return fmt.Errorf("Chunk file %v does not have the .chunk extension", chunk_fd.Name())
	}

	chunk_buf := make([]byte, api.SizeofCompressedChunk)
	for {
		_, err := chunk_fd.Read(chunk_buf)
		if err != nil {
			break
		}

		chunk := &api.CompressedChunk{}
		err = binary.Read(bytes.NewReader(chunk_buf), binary.LittleEndian, chunk)
		if err != nil {
			return err
		}

		compressed := make([]byte, chunk.CompressedLength)
		_, err = file_fd.Seek(chunk.ChunkOffset, os.SEEK_SET)
		if err != nil {
			return err
		}

		n, err := file_fd.Read(compressed)
		if err != nil || int64(n) != chunk.CompressedLength {
			break
		}

		uncompressed, err := utils.Uncompress(context.Background(), compressed)
		if err != nil {
			break
		}

		_, err = io.Copy(os.Stdout, bytes.NewReader(uncompressed))
		if err != nil {
			return err
		}
	}

	return nil
}

// Only register the filesystem accessor if we have a proper valid server config.
func initFilestoreAccessor(config_obj *config_proto.Config) error {
	if config_obj.Datastore != nil {
		fs_factory := file_store_accessor.NewFileStoreFileSystemAccessor(config_obj)
		accessors.Register(fs_factory)

		sparse_fs_factory := file_store_accessor.NewSparseFileStoreFileSystemAccessor(config_obj)
		accessors.Register(sparse_fs_factory)
	}
	return nil
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case fs_command_ls.FullCommand():
			err := doLS(*fs_command_ls_path, *fs_command_accessor)
			kingpin.FatalIfError(err, "%s", fs_command_ls.FullCommand())

		case fs_command_rm.FullCommand():
			err := doRM(*fs_command_rm_path, *fs_command_accessor)
			kingpin.FatalIfError(err, "%s", fs_command_rm.FullCommand())

		case fs_command_cp.FullCommand():
			err := doCp(*fs_command_cp_path,
				*fs_command_accessor, *fs_command_cp_outdir)
			kingpin.FatalIfError(err, "%s", fs_command_cp.FullCommand())

		case fs_command_cat.FullCommand():
			err := doCat(*fs_command_cat_path, *fs_command_accessor)
			kingpin.FatalIfError(err, "%s", fs_command_cat.FullCommand())

		case fs_command_zcat.FullCommand():
			err := doZCat(*fs_command_zcat_chunk_path, *fs_command_zcat_file_path)
			kingpin.FatalIfError(err, "%s", fs_command_zcat.FullCommand())

		default:
			return false
		}
		return true
	})
}
