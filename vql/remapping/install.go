package remapping

import (
	"context"
	"errors"
	"fmt"
	"runtime"

	"github.com/Velocidex/ordereddict"

	"www.velocidex.com/golang/velociraptor/accessors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

// Process a "mount" type remapping directive.
// Example:
//
//  remapping:
//  - type: mount
//    from:
//      accessor: file
//      prefix: /mnt/data
//    on:
//      accessor: file
//      prefix: C:\
//
// This reads like "mount directory from file:/mnt/data on file:/C:
// Means when VQL opens a path using accessor "file" in all paths
// below "C:/", the "file" accessor will be used on "/mnt/data"
// instead.
//
// You can also preconfigure the scope that will be used with the
// mounted accessor. This is needed for some accessors which look to
// the scope to configure them - for example the `ssh` accessor can be
// used with the following remapping file:
//
//remappings:
//- type: permissions
//  permissions:
//  - FILESYSTEM_READ
//
//- "type": mount
//  scope: |
//    LET SSH_CONFIG <= dict(hostname='localhost:22',
//      username='user',
//      private_key=read_file(filename='/home/user/.ssh/id_rsa'))
//
//  "from":
//    accessor: ssh
//    prefix: /
//  "on":
//    accessor: auto
//    prefix: /

func InstallMountPoints(
	ctx context.Context,
	config_obj *config_proto.Config,
	pristine_scope vfilter.Scope,
	manager accessors.DeviceManager,
	remappings []*config_proto.RemappingConfig,
	on_accessor string) error {

	if on_accessor == "" {
		on_accessor = "auto"
	}

	if len(remappings) == 0 {
		return nil
	}

	on_path_type := ""
	for _, remapping := range remappings {
		if remapping.On.PathType != "" {
			on_path_type = remapping.On.PathType
		}
	}

	// Build a mount filesystem
	root_path, err := getTypedOSPath(on_path_type, "")
	if err != nil {
		return err
	}
	root_fs := accessors.NewVirtualFilesystemAccessor(root_path)
	mount_fs := accessors.NewMountFileSystemAccessor(root_path, root_fs)

	// Apply all the mappings specified.
	for _, remapping := range remappings {
		from_accessor := remapping.From.Accessor
		if from_accessor == "" {
			from_accessor = "file"
		}

		// Evaluate VQL on pristine scope to prepare for accessor
		// configuration.
		evaluated_scope, err := evaluateScopeQuery(ctx,
			pristine_scope, remapping.Scope)
		if err != nil {
			return err
		}

		from_fs, err := accessors.GetDefaultDeviceManager(
			config_obj).GetAccessor(from_accessor, evaluated_scope)
		if err != nil {
			return err
		}

		// Where we mount the volume.
		on_path, err := getTypedOSPath(on_path_type, remapping.On.Prefix)
		if err != nil {
			return err
		}
		mount_directory := &accessors.VirtualFileInfo{
			IsDir_: true,
			Path:   on_path,
		}
		root_fs.SetVirtualFileInfo(mount_directory)
		from_path, err := getTypedOSPath(remapping.From.PathType,
			remapping.From.Prefix)
		if err != nil {
			return err
		}
		mount_fs.AddMapping(from_path, mount_directory.OSPath(), from_fs)
	}

	// Register the new accessor.
	manager.Register(accessors.DescribeAccessor(
		mount_fs, accessors.AccessorDescriptor{
			Name:        on_accessor,
			Description: fmt.Sprintf("Remapping %v", remappings),
		}))

	return nil
}

func getTypedOSPath(path_type string, path string) (*accessors.OSPath, error) {
	switch path_type {
	case "", "generic":
		return accessors.NewGenericOSPath(path)

	case "windows":
		return accessors.NewWindowsOSPath(path)

	case "registry":
		return accessors.NewWindowsRegistryPath(path)

	case "pathspec":
		return accessors.NewPathspecOSPath(path)

	case "ntfs":
		return accessors.NewWindowsNTFSPath(path)

	case "zip":
		return accessors.NewZipFilePath(path)

	default:
		return accessors.NewGenericOSPath(path)
	}
}

// Update the scope with the new device manager.
func ApplyRemappingOnScope(
	ctx context.Context,
	config_obj *config_proto.Config,
	pristine_scope vfilter.Scope,
	remapped_scope vfilter.Scope,
	manager accessors.DeviceManager,
	env *ordereddict.Dict,
	remappings []*config_proto.RemappingConfig) error {

	mounts := make(map[string][]*config_proto.RemappingConfig)

	for _, remapping := range remappings {
		switch remapping.Type {
		case "shadow":
			if remapping.From == nil || remapping.On == nil {
				return errors.New(
					"Invalid shadow mapping - both from and on " +
						"mount points should be specified.")
			}

			from_fs, err := accessors.GetDefaultDeviceManager(config_obj).
				GetAccessor(remapping.From.Accessor, pristine_scope)
			if err != nil {
				return err
			}

			// make a copy of the accessor that works on the remapped
			// scope.
			to_fs, err := from_fs.New(remapped_scope)
			if err != nil {
				return err
			}

			// Install on top of the manager
			manager.Register(accessors.DescribeAccessor(
				to_fs, accessors.AccessorDescriptor{
					Name:        remapping.On.Accessor,
					Description: "Shadowed",
				}))

		case "mount":
			if remapping.From == nil || remapping.On == nil {
				return errors.New(
					"Invalid mount mapping - both from and on " +
						"mount points should be specified.")
			}

			target_group := mounts[remapping.On.Accessor]
			target_group = append(target_group, remapping)
			mounts[remapping.On.Accessor] = target_group

		case "permissions":

		case "impersonation":
			remapped_scope.AppendPlugins(NewMockerPlugin(
				"info", []types.Any{
					ordereddict.NewDict().
						Set("Hostname", remapping.Hostname).
						Set("Fqdn", remapping.Hostname).
						Set("Uptime", "").
						Set("BootTime", "").
						Set("Procs", "").
						Set("OS", remapping.Os).
						Set("Platform", "").
						Set("PlatformFamily", "").
						Set("PlatformVersion", "").
						Set("KernelVersion", "").
						Set("VirtualizationSystem", "").
						Set("VirtualizationRole", "").
						Set("HostID", "").
						Set("Exe", "").
						Set("Architecture", runtime.GOARCH).
						Set("IsAdmin", true),
				}))
			disablePlugins(remapped_scope, remapping)

		default:
			return fmt.Errorf("Unknown remapping type: %v", remapping.Type)
		}
	}

	for to_accessor, remappings := range mounts {
		err := InstallMountPoints(ctx, config_obj, pristine_scope,
			manager, remappings, to_accessor)
		if err != nil {
			return err
		}
	}

	installExpandMock(pristine_scope, remappings)

	return nil
}

func evaluateScopeQuery(
	ctx context.Context, scope vfilter.Scope, query string) (vfilter.Scope, error) {
	if query == "" {
		return scope, nil
	}

	sub_scope := scope.Copy()

	vqls, err := vfilter.MultiParse(query)
	if err != nil {
		return nil, fmt.Errorf(
			"While evaluating remapping scope: %w", err)
	}

	for _, vql := range vqls {
		if vql.Let == "" {
			return nil, fmt.Errorf(
				"While evaluating remapping scope: Only LET statements allowed in this context")
		}
		for _ = range vql.Eval(ctx, sub_scope) {
		}
	}

	return sub_scope, nil
}
