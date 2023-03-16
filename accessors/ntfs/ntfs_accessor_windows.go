// +build windows

package ntfs

import (
	"context"
	"time"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/utils"
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

func (self *WindowsNTFSFileSystemAccessor) Lstat(path string) (accessors.FileInfo, error) {
	// Parse the path into an OSPath
	os_path, err := self.ParsePath(path)
	if err != nil {
		return nil, err
	}

	return self.LstatWithOSPath(os_path)
}

func (self *WindowsNTFSFileSystemAccessor) LstatWithOSPath(os_path *accessors.OSPath) (accessors.FileInfo, error) {

	// Calling an LStat on the device shall return file info about the
	// device itself (including size). e.g. Lstat("\\C:\") -> info about the volume.
	if len(os_path.Components) == 1 {
		// Try to match the device to the component required. This can
		// be either a VSS volume or a Logical disk volume.
		devices, err := discoverVSS()
		if err != nil {
			return nil, err
		}

		for _, d := range devices {
			if d.Name() == os_path.Components[0] {
				return d, nil
			}
		}
		devices, err = discoverLogicalDisks()
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
			device_path, err := accessors.NewWindowsNTFSPath(device_name)
			if err != nil {
				return nil, err
			}
			virtual_directory := &accessors.VirtualFileInfo{
				IsDir_: true,
				Path:   device_path,
				Size_:  0, // WMI does not give the original volume size
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
			device_path, err := accessors.NewWindowsNTFSPath("\\\\.\\" + device_name)
			if err != nil {
				return nil, err
			}
			virtual_directory := &accessors.VirtualFileInfo{
				IsDir_: true,
				Size_:  utils.GetInt64(row, "Size"),
				Path:   device_path,
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
	root_path, _ := accessors.NewWindowsNTFSPath("")
	root_fs := accessors.NewVirtualFilesystemAccessor(root_path)

	result := &WindowsNTFSFileSystemAccessor{
		MountFileSystemAccessor: accessors.NewMountFileSystemAccessor(
			root_path, root_fs),
		age: time.Now(),
	}

	vss, err := discoverVSS()
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

	logical, err := discoverLogicalDisks()
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

func init() {
	// For backwards compatibility.
	accessors.Register("lazy_ntfs", &WindowsNTFSFileSystemAccessor{},
		`Access the NTFS filesystem by parsing NTFS structures.`)

	accessors.Register("ntfs", &WindowsNTFSFileSystemAccessor{},
		`Access the NTFS filesystem by parsing NTFS structures.`)
}
