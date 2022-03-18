package remapping

import (
	"context"
	"fmt"

	"github.com/Velocidex/ordereddict"
	errors "github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/accessors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/vfilter"
)

// Process a "mount" type remapping directive.
// Example:
//
//  - type: mount
//    from:
//      accessor: file
//      prefix: /shared/deaddisk/c/
//    on:
//      accessor: file
//      prefix: C:\
//
// This reads like "mount directory from file:/mnt/data on file:/
// Means when VQL opens a path using accessor "file" in all paths
// below "/", the "file" accessor will be used on "/mnt/data" instead.
func InstallMountPoints(
	scope vfilter.Scope,
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

		from_fs, err := accessors.GlobalDeviceManager.GetAccessor(
			from_accessor, scope)
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
	manager.Register(on_accessor, mount_fs,
		fmt.Sprintf("Remapping %v", remappings))

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

	default:
		return accessors.NewGenericOSPath(path)
	}
}

// Update the scope with the new device manager.
func ApplyRemappingOnScope(
	ctx context.Context,
	scope vfilter.Scope,
	manager accessors.DeviceManager,
	env *ordereddict.Dict,
	remappings []*config_proto.RemappingConfig) error {

	mounts := make(map[string][]*config_proto.RemappingConfig)

	for _, remapping := range remappings {
		switch remapping.Type {
		case "shadow":
			if remapping.From == nil || remapping.On == nil ||
				remapping.From.Accessor == "" ||
				remapping.On.Accessor == "" {
				return errors.New(
					"Invalid shadow mapping - both from and on " +
						"mount points should be specified.")
			}

			from_fs, err := accessors.GlobalDeviceManager.GetAccessor(
				remapping.From.Accessor, scope)
			if err != nil {
				return err
			}

			// Install on top of the manager
			manager.Register(remapping.On.Accessor, from_fs, "Shadowed")

		case "mount":
			if remapping.From == nil || remapping.On == nil ||
				remapping.From.Accessor == "" ||
				remapping.On.Accessor == "" {
				return errors.New(
					"Invalid mount mapping - both from and on " +
						"mount points should be specified.")
			}

			target_group := mounts[remapping.On.Accessor]
			target_group = append(target_group, remapping)
			mounts[remapping.On.Accessor] = target_group

		case "permissions":

		case "impersonation":
			var mocker_ctx *MockingScopeContext
			mocker_ctx_any, pres := env.Get(constants.SCOPE_MOCK)
			if pres {
				mocker_ctx, _ = mocker_ctx_any.(*MockingScopeContext)
			}
			if mocker_ctx == nil {
				mocker_ctx = &MockingScopeContext{}
			}
			env.Set(constants.SCOPE_MOCK, mocker_ctx)
			mock_plugin := NewMockerPlugin(
				"info", []*ordereddict.Dict{
					ordereddict.NewDict().
						Set("OS", remapping.Os).
						Set("Hostname", remapping.Hostname),
				})

			mocker_ctx.AddPlugin(mock_plugin)
			scope.AppendPlugins(mock_plugin)

		default:
			return fmt.Errorf("Unknown remapping type: %v", remapping.Type)
		}
	}

	for to_accessor, remappings := range mounts {
		err := InstallMountPoints(scope, manager, remappings, to_accessor)
		if err != nil {
			return err
		}
	}

	return nil
}
