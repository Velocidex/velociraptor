package filesystems

import (
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/vql/windows/wmi"
)

func discoverVSS() ([]glob.FileInfo, error) {
	result := []glob.FileInfo{}

	shadow_volumes, err := wmi.Query(
		"SELECT DeviceObject, VolumeName, InstallDate, "+
			"OriginatingMachine from Win32_ShadowCopy",
		"ROOT\\CIMV2")
	if err == nil {
		for _, row := range shadow_volumes {
			size_str, _ := row.GetString("Size")
			size, _ := strconv.Atoi(size_str)
			device_name, pres := row.GetString("DeviceObject")
			if pres {
				virtual_directory := glob.NewVirtualDirectoryPath(
					device_name, row, int64(size), os.ModeDir)
				result = append(result, virtual_directory)
			}
		}
	}

	return result, nil
}

func discoverLogicalDisks() ([]glob.FileInfo, error) {
	result := []glob.FileInfo{}

	shadow_volumes, err := wmi.Query(
		"SELECT DeviceID, Description, VolumeName, FreeSpace, "+
			"Size, SystemName, VolumeSerialNumber "+
			"from Win32_LogicalDisk WHERE FileSystem = 'NTFS'",
		"ROOT\\CIMV2")
	if err == nil {
		for _, row := range shadow_volumes {
			size_str, _ := row.GetString("Size")
			size, _ := strconv.Atoi(size_str)

			device_name, pres := row.GetString("DeviceID")
			if pres {
				virtual_directory := glob.NewVirtualDirectoryPath(
					"\\\\.\\"+device_name, row, int64(size), os.ModeDir)
				result = append(result, virtual_directory)
			}
		}
	}

	return result, nil
}

func (self *NTFSFileSystemAccessor) GetRoot(path string) (
	device string, subpath string, err error) {
	return paths.GetDeviceAndSubpath(path)
}
