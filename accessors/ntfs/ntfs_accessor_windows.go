//go:build windows
// +build windows

package ntfs

import (
	"context"
	"time"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/file"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/constants"
	"www.velocidex.com/golang/vfilter"
)

const (
	NTFS_TAG = "$__NTFS_Accessor"
)

type WindowsNTFSFileSystemAccessor struct {
	*accessors.MountFileSystemAccessor
	age time.Time
}

func (self *WindowsNTFSFileSystemAccessor) Lstat(path string) (accessors.FileInfo, error) {
	// Parse the path into an OSPath
	os_path, err := self.ParsePath(path)
	if err != nil {
		return nil, err
	}

	return self.LstatWithOSPath(os_path)
}

func (self *WindowsNTFSFileSystemAccessor) LstatWithOSPath(
	os_path *accessors.OSPath) (accessors.FileInfo, error) {

	defer Instrument("LstatWithOSPath")()

	err := file.CheckPrefix(os_path)
	if err != nil {
		return nil, err
	}

	// Calling an LStat on the device shall return file info about the
	// device itself (including size). e.g. Lstat("\\C:\") -> info about the volume.
	if len(os_path.Components) == 1 {
		// Try to match the device to the component required. This can
		// be either a VSS volume or a Logical disk volume.
		devices, err := Cache.DiscoverVSS()
		if err != nil {
			return nil, err
		}

		for _, d := range devices {
			if d.Name() == os_path.Components[0] {
				return d, nil
			}
		}
		devices, err = Cache.DiscoverLogicalDisks()
		if err != nil {
			return nil, err
		}

		for _, d := range devices {
			if d.Name() == os_path.Components[0] {
				return d, nil
			}
		}
	}

	return self.MountFileSystemAccessor.LstatWithOSPath(os_path)
}

func (self *WindowsNTFSFileSystemAccessor) New(
	scope vfilter.Scope) (accessors.FileSystemAccessor, error) {

	// Cache the ntfs accessor for the life of the query.
	cache_time := constants.GetNTFSCacheTime(context.Background(), scope)
	root_scope := vql_subsystem.GetRootScope(scope)
	cached_accessor, ok := vql_subsystem.CacheGet(
		root_scope, NTFS_TAG).(*WindowsNTFSFileSystemAccessor)

	// Ignore the filesystem if it is too old - drives may have been
	// added or removed.
	if ok && cached_accessor.age.Add(cache_time).After(time.Now()) {
		return cached_accessor, nil
	}

	// Build a virtual filesystem that mounts the various NTFS volumes on it.
	root_path, _ := accessors.NewWindowsNTFSPath("")
	root_fs := accessors.NewVirtualFilesystemAccessor(root_path)

	result := &WindowsNTFSFileSystemAccessor{
		MountFileSystemAccessor: accessors.NewMountFileSystemAccessor(
			root_path, root_fs),
		age: time.Now(),
	}

	vss, err := Cache.DiscoverVSS()
	if err == nil {
		for _, fi := range vss {
			root_fs.SetVirtualFileInfo(fi)
			result.AddMapping(
				root_path, // Mount at the root of the filesystem
				fi.OSPath(),
				NewNTFSFileSystemAccessor(
					root_scope, root_path, fi.OSPath(), "file"))
		}
	}

	logical, err := Cache.DiscoverLogicalDisks()
	if err == nil {
		for _, fi := range logical {
			root_fs.SetVirtualFileInfo(fi)
			result.AddMapping(
				root_path,
				fi.OSPath(),
				NewNTFSFileSystemAccessor(
					root_scope, root_path, fi.OSPath(), "file"))
		}
	}

	vql_subsystem.CacheSet(root_scope, NTFS_TAG, result)
	return result, nil
}

func (self WindowsNTFSFileSystemAccessor) Describe() *accessors.AccessorDescriptor {
	return &accessors.AccessorDescriptor{
		Name:        "ntfs",
		Description: `Access the NTFS filesystem by parsing NTFS structures.`,
	}
}

func init() {
	// For backwards compatibility. It is the same as "ntfs"
	accessors.Register(accessors.DescribeAccessor(
		&WindowsNTFSFileSystemAccessor{},
		accessors.AccessorDescriptor{
			Name:        "lazy_ntfs",
			Description: `Access the NTFS filesystem by parsing NTFS structures.`,
		}))
	accessors.Register(&WindowsNTFSFileSystemAccessor{})
}
