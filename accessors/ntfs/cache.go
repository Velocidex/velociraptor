//go:build windows
// +build windows

package ntfs

import (
	"sync"
	"time"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/windows/wmi"
)

var (
	Cache = WMICache{}
)

type WMICache struct {
	mu            sync.Mutex
	last          time.Time
	logical_disks []*accessors.VirtualFileInfo
	vss           []*accessors.VirtualFileInfo
}

func (self *WMICache) realDiscoverVSS() ([]*accessors.VirtualFileInfo, error) {
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

func (self *WMICache) realDiscoverLogicalDisks() ([]*accessors.VirtualFileInfo, error) {
	result := []*accessors.VirtualFileInfo{}
	shadow_volumes, err := wmi.Query(
		"SELECT DeviceID, Description, VolumeName, FreeSpace, "+
			"Size, SystemName, VolumeSerialNumber "+
			"from Win32_LogicalDisk WHERE FileSystem = 'NTFS'",
		"ROOT\\CIMV2")
	if err != nil {
		return nil, err
	}

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

func (self *WMICache) maybeUpdateCache() error {
	// Result is not too old - return it.
	now := utils.GetTime().Now()
	if self.last.Add(time.Minute).After(now) {
		return nil
	}

	logical_disks, err := self.realDiscoverLogicalDisks()
	if err != nil {
		return err
	}

	vss, err := self.realDiscoverVSS()
	if err != nil {
		return err
	}

	self.last = now
	self.logical_disks = logical_disks
	self.vss = vss

	return nil
}

func (self *WMICache) DiscoverLogicalDisks() ([]*accessors.VirtualFileInfo, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	err := self.maybeUpdateCache()
	if err != nil {
		return nil, err
	}

	var result []*accessors.VirtualFileInfo
	for _, r := range self.logical_disks {
		result = append(result, r)
	}
	return result, nil
}

func (self *WMICache) DiscoverVSS() ([]*accessors.VirtualFileInfo, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	err := self.maybeUpdateCache()
	if err != nil {
		return nil, err
	}

	var result []*accessors.VirtualFileInfo
	for _, r := range self.vss {
		result = append(result, r)
	}
	return result, nil
}
