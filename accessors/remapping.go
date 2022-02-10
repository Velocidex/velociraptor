package accessors

import (
	"context"
	"fmt"

	"github.com/Velocidex/ordereddict"
	errors "github.com/pkg/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
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
func InstallMountPoints(manager DeviceManager,
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

	scope := vql_subsystem.MakeScope().AppendVars(ordereddict.NewDict().
		Set(vql_subsystem.ACL_MANAGER_VAR, vql_subsystem.NullACLManager{}))

	// Build a mount filesystem
	root_fs := NewVirtualFilesystemAccessor()
	mount_fs := NewMountFileSystemAccessor(
		getTypedOSPath(on_path_type, ""), root_fs)

	// Apply all the mappings specified.
	for _, remapping := range remappings {
		from_accessor := remapping.From.Accessor
		if from_accessor == "" {
			from_accessor = "file"
		}

		from_fs, err := GlobalDeviceManager.GetAccessor(from_accessor, scope)
		if err != nil {
			return err
		}

		// Where we mount the volume.
		mount_directory := &VirtualFileInfo{
			IsDir_: true,
			Path:   getTypedOSPath(on_path_type, remapping.On.Prefix),
		}
		root_fs.SetVirtualFileInfo(mount_directory)
		mount_fs.AddMapping(
			getTypedOSPath(remapping.From.PathType, remapping.From.Prefix),
			mount_directory.OSPath(), from_fs)
	}

	// Register the new accessor.
	manager.Register(on_accessor, mount_fs,
		fmt.Sprintf("Remapping %v", remappings))

	return nil
}

func getTypedOSPath(path_type string, path string) *OSPath {
	switch path_type {
	case "", "generic":
		return NewGenericOSPath(path)

	case "windows":
		return NewWindowsOSPath(path)

	case "registry":
		return NewWindowsRegistryPath(path)

	case "pathspec":
		return NewPathspecOSPath(path)

	case "ntfs":
		return NewWindowsNTFSPath(path)

	default:
		return NewGenericOSPath(path)
	}
}

// Update the scope with the new device manager.
func ApplyRemappingOnScope(
	ctx context.Context,
	manager DeviceManager,
	remappings []*config_proto.RemappingConfig) error {

	mounts := make(map[string][]*config_proto.RemappingConfig)

	for _, remapping := range remappings {
		if remapping.Type == "mount" {
			if remapping.From == nil || remapping.On == nil {
				return errors.New(
					"Invalid mount mapping - both from and on " +
						"mount points should be specified.")
			}

			target_group := mounts[remapping.On.Accessor]
			target_group = append(target_group, remapping)
			mounts[remapping.On.Accessor] = target_group
			continue
		}

		return fmt.Errorf("Unknown remapping type: %v", remapping.Type)
	}

	for to_accessor, remappings := range mounts {
		err := InstallMountPoints(manager, remappings, to_accessor)
		if err != nil {
			return err
		}
	}

	return nil
}
