package filesystems

import (
	"strings"

	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/remapping"
	"www.velocidex.com/golang/velociraptor/vql/filesystem/ntfs"
	"www.velocidex.com/golang/velociraptor/vql/windows/wmi"
	"www.velocidex.com/golang/vfilter"
)

type WindowsNTFSFileSystemAccessor struct {
	*remapping.MountFileSystemAccessor

	root_fs *remapping.VirtualFilesystemAccessor
}

func (self WindowsNTFSFileSystemAccessor) PathJoin(x, y string) string {
	return x + "\\" + strings.TrimLeft(y, "\\")
}

func discoverVSS() ([]*remapping.VirtualFileInfo, error) {
	shadow_volumes, err := wmi.Query(
		"SELECT DeviceObject, VolumeName, InstallDate, "+
			"OriginatingMachine from Win32_ShadowCopy",
		"ROOT\\CIMV2")
	if err != nil {
		return nil, err
	}

	result := []*remapping.VirtualFileInfo{}
	for _, row := range shadow_volumes {
		device_name, pres := row.GetString("DeviceObject")
		if pres {
			virtual_directory := &remapping.VirtualFileInfo{
				IsDir_:      true,
				Components_: []string{device_name},
				FullPath_:   device_name,
				Data_:       row,
			}
			result = append(result, virtual_directory)
		}
	}

	return result, nil
}

func discoverLogicalDisks() ([]*remapping.VirtualFileInfo, error) {
	shadow_volumes, err := wmi.Query(
		"SELECT DeviceID, Description, VolumeName, FreeSpace, "+
			"Size, SystemName, VolumeSerialNumber "+
			"from Win32_LogicalDisk WHERE FileSystem = 'NTFS'",
		"ROOT\\CIMV2")
	if err != nil {
		return nil, err
	}

	result := []*remapping.VirtualFileInfo{}
	for _, row := range shadow_volumes {
		device_name, pres := row.GetString("DeviceID")
		if pres {
			virtual_directory := &remapping.VirtualFileInfo{
				IsDir_:      true,
				Components_: []string{"\\\\.\\" + device_name},
				FullPath_:   "\\\\.\\" + device_name,
				Data_:       row,
			}
			result = append(result, virtual_directory)
		}
	}

	return result, nil
}

func (self *WindowsNTFSFileSystemAccessor) New(
	scope vfilter.Scope) (glob.FileSystemAccessor, error) {
	root_fs := remapping.NewVirtualFilesystemAccessor()
	result := &WindowsNTFSFileSystemAccessor{
		MountFileSystemAccessor: remapping.NewMountFileSystemAccessor(root_fs),
		root_fs:                 root_fs,
	}

	vss, err := discoverVSS()
	if err == nil {
		for _, fi := range vss {
			root_fs.SetVirtualFileInfo(fi)
		}
	}

	logical, err := discoverLogicalDisks()
	if err == nil {
		for _, fi := range logical {
			root_fs.SetVirtualFileInfo(fi)
			result.AddMappingComponents(
				[]string{}, fi.Components_,
				ntfs.NewNTFSFileSystemAccessor(scope, fi.Name(), "file"))
		}
	}

	return result, nil
}

func init() {
	glob.Register("ntfs", &WindowsNTFSFileSystemAccessor{},
		`Access the NTFS filesystem by parsing NTFS structures.`)
}
