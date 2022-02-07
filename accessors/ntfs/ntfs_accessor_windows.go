// +build windows

package ntfs

import (
	"context"
	"time"

	"www.velocidex.com/golang/velociraptor/accessors"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/constants"
	"www.velocidex.com/golang/velociraptor/vql/windows/wmi"
	"www.velocidex.com/golang/vfilter"
)

const (
	NTFS_TAG = "$__NTFS_Accessor"
)

type WindowsNTFSFileSystemAccessor struct {
	*accessors.MountFileSystemAccessor
	age time.Time
}

func discoverVSS() ([]*accessors.VirtualFileInfo, error) {
	shadow_volumes, err := wmi.Query(
		"SELECT DeviceObject, VolumeName, InstallDate, "+
			"OriginatingMachine from Win32_ShadowCopy",
		"ROOT\\CIMV2")
	if err != nil {
		return nil, err
	}

	result := []*accessors.VirtualFileInfo{}
	for _, row := range shadow_volumes {
		device_name, pres := row.GetString("DeviceObject")
		if pres {
			virtual_directory := &accessors.VirtualFileInfo{
				IsDir_: true,
				Path:   accessors.NewWindowsNTFSPath(device_name),
				Data_:  row,
			}
			result = append(result, virtual_directory)
		}
	}

	return result, nil
}

func discoverLogicalDisks() ([]*accessors.VirtualFileInfo, error) {
	shadow_volumes, err := wmi.Query(
		"SELECT DeviceID, Description, VolumeName, FreeSpace, "+
			"Size, SystemName, VolumeSerialNumber "+
			"from Win32_LogicalDisk WHERE FileSystem = 'NTFS'",
		"ROOT\\CIMV2")
	if err != nil {
		return nil, err
	}

	result := []*accessors.VirtualFileInfo{}
	for _, row := range shadow_volumes {
		device_name, pres := row.GetString("DeviceID")
		if pres {
			virtual_directory := &accessors.VirtualFileInfo{
				IsDir_: true,
				Path:   accessors.NewWindowsNTFSPath("\\\\.\\" + device_name),
				Data_:  row,
			}
			result = append(result, virtual_directory)
		}
	}

	return result, nil
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
	root_fs := accessors.NewVirtualFilesystemAccessor()
	result := &WindowsNTFSFileSystemAccessor{
		MountFileSystemAccessor: accessors.NewMountFileSystemAccessor(
			accessors.NewWindowsNTFSPath(""), root_fs),
		age: time.Now(),
	}

	vss, err := discoverVSS()
	if err == nil {
		for _, fi := range vss {
			root_fs.SetVirtualFileInfo(fi)
			result.AddMapping(
				accessors.NewWindowsNTFSPath(""), // Mount at the root of the filesystem
				fi.OSPath(),
				NewNTFSFileSystemAccessor(root_scope, fi.FullPath(), "file"))
		}
	}

	logical, err := discoverLogicalDisks()
	if err == nil {
		for _, fi := range logical {
			root_fs.SetVirtualFileInfo(fi)
			result.AddMapping(
				accessors.NewWindowsNTFSPath(""),
				fi.OSPath(),
				NewNTFSFileSystemAccessor(root_scope, fi.FullPath(), "file"))
		}
	}

	vql_subsystem.CacheSet(root_scope, NTFS_TAG, result)
	return result, nil
}

func init() {
	// For backwards compatibility.
	accessors.Register("lazy_ntfs", &WindowsNTFSFileSystemAccessor{},
		`Access the NTFS filesystem by parsing NTFS structures.`)

	accessors.Register("ntfs", &WindowsNTFSFileSystemAccessor{},
		`Access the NTFS filesystem by parsing NTFS structures.`)
}
