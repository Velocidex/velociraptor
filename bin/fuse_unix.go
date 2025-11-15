//go:build !windows && !freebsd
// +build !windows,!freebsd

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

	fuse_zip_accessor = fuse_command.Flag("accessor", "The accessor to use (default collector)").
				Default("collector").String()

	fuse_zip_prefix = fuse_command.Flag("prefix",
		"Export all files below this directory in the zip file").
		Default("/").String()

	fuse_files = fuse_zip_command.Arg("files", "list of zip files to mount").
			Required().Strings()

	fuse_options_map_device_names_to_letters = fuse_zip_command.Flag(
		"map_device_names_to_letters", "Convert raw device names to drive letters").
		Default("true").Bool()

	fuse_options_strip_colons_on_drive_letters = fuse_zip_command.Flag(
		"strip_colons_on_drive_letters", "Remove the : on drive letters").
		Default("true").Bool()

	fuse_options_unix_path_escaping = fuse_zip_command.Flag("unix_path_escaping",
		"If set we escape only few characters in file names otherwise escape windows compatible chars").
		Bool()

	fuse_options_emulate_timestamps = fuse_zip_command.Flag("emulate_timestamps",
		"If set emulate timestamps for common artifacts like Windows.Triage.Targets.").
		Default("true").Bool()

	fuse_options_merge_accessors = fuse_zip_command.Flag("merge_accessors",
		"If set merge all the accessors into the same "+
			"directory (implied --map_device_names_to_letters).").
		Default("true").Bool()
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
		err = ospath.SetPathSpec(
			&accessors.PathSpec{
				DelegatePath: filename,
				Path:         *fuse_zip_prefix,
			})
		if err != nil {
			return err
		}
		paths = append(paths, ospath)
	}

	accessor_fs, err := fuse.NewAccessorFuseFS(
		ctx, config_obj, accessor,
		&fuse.Options{
			MapDeviceNamesToLetters: *fuse_options_map_device_names_to_letters ||
				*fuse_options_merge_accessors,
			MapDriveNamesToLetters:     *fuse_options_strip_colons_on_drive_letters,
			UnixCompatiblePathEscaping: *fuse_options_unix_path_escaping,
			EmulateTimestamps:          *fuse_options_emulate_timestamps,
			MergeAllAccessors:          *fuse_options_merge_accessors,
		}, paths)
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
