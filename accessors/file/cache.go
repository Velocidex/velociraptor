//go:build windows
// +build windows

package file

import (
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/files"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/windows/wmi"
	"www.velocidex.com/golang/vfilter"
)

var (
	Cache             = WMICache{}
	FILE_ACCESSOR_TAG = "$__file_accessor"
)

type WMICache struct {
	mu            sync.Mutex
	last          time.Time
	logical_disks []*accessors.VirtualFileInfo
}

func (self *WMICache) maybeUpdateCache() error {
	// Result is not too old - return it.
	now := utils.GetTime().Now()
	if self.last.Add(time.Minute).After(now) {
		return nil
	}

	logical_disks, err := self.realDiscoverDriveLetters()
	if err != nil {
		return err
	}

	self.last = now
	self.logical_disks = logical_disks

	return nil
}

func (self *WMICache) DiscoverDriveLetters() ([]accessors.FileInfo, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	err := self.maybeUpdateCache()
	if err != nil {
		return nil, err
	}

	var result []accessors.FileInfo
	for _, i := range self.logical_disks {
		result = append(result, i)
	}

	return result, nil
}

func (self *WMICache) realDiscoverDriveLetters() ([]*accessors.VirtualFileInfo, error) {
	var result []*accessors.VirtualFileInfo

	shadow_volumes, err := wmi.Query(
		"SELECT DeviceID, Description, VolumeName, FreeSpace, "+
			"Size, SystemName, VolumeSerialNumber "+
			"from Win32_LogicalDisk",
		"ROOT\\CIMV2")
	if err == nil {
		for _, row := range shadow_volumes {
			size := utils.GetInt64(row, "Size")
			device_name, pres := row.GetString("DeviceID")
			if pres {
				device_path, err := accessors.NewWindowsOSPath(device_name)
				if err != nil {
					return nil, err
				}

				err = CheckPrefix(device_path)
				if err != nil {
					continue
				}

				result = append(result, &accessors.VirtualFileInfo{
					IsDir_: true,
					Size_:  size,
					Data_:  row,
					Path:   device_path,
				})
			}
		}
	}

	return result, nil
}

func getDeviceReader(scope vfilter.Scope,
	device_name string) (accessors.ReadSeekCloser, error) {
	var device_cache *ordereddict.Dict

	if !strings.HasPrefix(device_name, "\\\\") {
		device_name = "\\\\.\\" + device_name
	}

	device_cache_any := vql_subsystem.CacheGet(scope, FILE_ACCESSOR_TAG)
	device_cache, ok := device_cache_any.(*ordereddict.Dict)
	if !ok || device_cache == nil {
		device_cache = ordereddict.NewDict()
		vql_subsystem.CacheSet(scope, FILE_ACCESSOR_TAG, device_cache)
	}

	reader_any, ok := device_cache.Get(device_name)
	if ok {
		reader, ok := reader_any.(accessors.ReadSeekCloser)
		if ok {
			return reader, nil
		}
	}

	defer Instrument("RawDevice")()

	file, err := os.Open(device_name)
	if err != nil {
		return nil, err
	}

	files.Add(device_name)
	// Only close the file when the scope is destroyed.
	vql_subsystem.GetRootScope(scope).AddDestructor(func() {
		file.Close()
		files.Remove(device_name)
	})

	// Need to read the raw device in pagesize sizes
	reader, err := ntfs.NewPagedReader(file, 0x10000, 100)
	if err != nil {
		return nil, err
	}

	res := utils.NewReadSeekReaderAdapter(reader, nil)

	// Try to figure out the size - not necessary but in case we
	// can we can limit readers to this size.
	stat, err1 := os.Lstat(device_name)
	if err1 == nil {
		res.SetSize(stat.Size())
	}

	device_cache.Set(device_name, res)

	return res, nil
}
