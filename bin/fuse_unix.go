//go:build !windows
// +build !windows

package main

import (
	"fmt"
	"log"
	"time"

	"github.com/Velocidex/ordereddict"
	kingpin "github.com/alecthomas/kingpin/v2"
	"github.com/hanwen/go-fuse/v2/fs"
	"www.velocidex.com/golang/velociraptor/accessors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/startup"
	"www.velocidex.com/golang/velociraptor/tools/fuse"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
)

var (
	fuse_command = app.Command("fuse", "Use fuse mounts")

	fuse_zip_command = fuse_command.Command("container", "Mount ZIP containers over fuse")

	fuse_directory = fuse_zip_command.Arg("directory", "A directory to mount on").
			Required().String()

	fuse_tmp_dir = fuse_zip_command.Flag("tmpdir",
		"A temporary directory to use (if not specified we use our own tempdir)").
		String()

	fuse_zip_accessor = fuse_command.Flag("accessor", "The accessor to use (default container)").
				Default("collector").String()

	fuse_zip_prefix = fuse_command.Flag("prefix", "Export all files below this directory in the zip file").
			Default("/").String()

	fuse_files = fuse_zip_command.Arg("files", "list of zip files to mount").
			Required().Strings()
)

func doFuseZip() error {
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

	logger := &LogWriter{config_obj: sm.Config}
	builder := services.ScopeBuilder{
		Config:     sm.Config,
		ACLManager: acl_managers.NewRoleACLManager(sm.Config, "administrator"),
		Logger:     log.New(logger, "", 0),
		Env:        ordereddict.NewDict(),
	}

	manager, err := services.GetRepositoryManager(builder.Config)
	if err != nil {
		return err
	}

	scope := manager.BuildScope(builder)
	defer scope.Close()

	accessor, err := accessors.GetAccessor(*fuse_zip_accessor, scope)
	if err != nil {
		return err
	}

	paths := make([]*accessors.OSPath, 0, len(*fuse_files))

	for _, filename := range *fuse_files {
		ospath, err := accessor.ParsePath("")
		if err != nil {
			return fmt.Errorf("Parsing %v with accessor %v: %v",
				filename, *fuse_zip_accessor, err)
		}
		ospath.SetPathSpec(
			&accessors.PathSpec{
				DelegatePath: filename,
				Path:         *fuse_zip_prefix,
			})

		paths = append(paths, ospath)
	}

	accessor_fs, err := fuse.NewAccessorFuseFS(
		ctx, config_obj, accessor, paths)
	if err != nil {
		return err
	}
	defer accessor_fs.Close()

	ttl := time.Duration(5 * time.Second)

	opts := &fs.Options{
		AttrTimeout:  &ttl,
		EntryTimeout: &ttl,
	}

	server, err := fs.Mount(*fuse_directory, accessor_fs, opts)
	kingpin.FatalIfError(err, "Mounting fuse")

	go func() {
		defer cancel()

		server.Wait()
	}()

	<-ctx.Done()

	return nil
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case fuse_zip_command.FullCommand():
			FatalIfError(fuse_zip_command, doFuseZip)

		default:
			return false
		}
		return true
	})
}
