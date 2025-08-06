//go:build linux
// +build linux

/*
 This accessor is similar to the Windows ntfs accessor. It
 automatically enumerates the mount points and attaches a raw ext4
 mount to each mounted device.

 Users can use the same path as is presented on the real system, but
 the raw ext4 partitions will be parsed instead.

 This accessor is only available under linux.
*/

package ext4

import (
	"context"

	"www.velocidex.com/golang/velociraptor/accessors"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/psutils"
	"www.velocidex.com/golang/vfilter"
)

const (
	EXT4_Tag = "_EXT4_Tag"
)

type LinuxExt4FileSystemAccessor struct {
	*accessors.MountFileSystemAccessor
}

func (self LinuxExt4FileSystemAccessor) GetVirtualFS(scope vfilter.Scope) (
	*accessors.MountFileSystemAccessor, error) {

	mount_fs, ok := vql_subsystem.CacheGet(scope, EXT4_Tag).(*accessors.MountFileSystemAccessor)
	if ok {
		return mount_fs, nil
	}

	root_path := accessors.MustNewLinuxOSPath("/")
	virtual_fs := accessors.NewVirtualFilesystemAccessor(root_path)
	mount_fs = accessors.NewMountFileSystemAccessor(root_path, virtual_fs)

	vql_subsystem.CacheSet(scope, EXT4_Tag, mount_fs)

	ctx := context.Background()
	partitions, err := psutils.PartitionsWithContext(ctx)
	if err != nil {
		return nil, err
	}

	for _, p := range partitions {
		if p.Fstype != "ext4" {
			continue
		}

		// Mount the partition
		target, err := accessors.NewLinuxOSPath(p.Mountpoint)
		if err != nil {
			continue
		}

		device, err := accessors.NewLinuxOSPath(p.Device)
		if err != nil {
			continue
		}

		scope.Log("ext4: Adding mapping to %v on %v",
			device, p.Mountpoint)

		// Need to use raw_file to be able to open a device file.
		mount_fs.AddMapping(root_path,
			target,
			NewExt4FileSystemAccessor(
				scope, root_path, device, "raw_file"))
	}

	return mount_fs, nil
}

func (self LinuxExt4FileSystemAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {

	mount_fs, err := self.GetVirtualFS(scope)
	// Create a new cache in the scope.
	return &LinuxExt4FileSystemAccessor{
		MountFileSystemAccessor: mount_fs,
	}, err
}

func (self LinuxExt4FileSystemAccessor) Describe() *accessors.AccessorDescriptor {
	return &accessors.AccessorDescriptor{
		Name:        "ext4",
		Description: `Access files by parsing the raw ext4 filesystems.`,
	}
}

func init() {
	accessors.Register(&LinuxExt4FileSystemAccessor{})
}
