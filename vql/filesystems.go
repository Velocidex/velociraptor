package vql

import (
	"github.com/shirou/gopsutil/disk"
	"www.velocidex.com/golang/vfilter"
)

type ExtendedFileSystemInfo struct {
	Partition disk.PartitionStat
}

func (self ExtendedFileSystemInfo) Usage() *disk.UsageStat {
	usage, err := disk.Usage(self.Partition.Mountpoint)
	if err != nil {
		return nil
	}

	return usage
}

func (self ExtendedFileSystemInfo) SerialNumber() string {
	return disk.GetDiskSerialNumber(self.Partition.Device)
}

func MakePatritionsPlugin() vfilter.GenericListPlugin {
	return vfilter.GenericListPlugin{
		PluginName: "partitions",
		Function: func(
			scope *vfilter.Scope,
			args *vfilter.Dict) []vfilter.Row {
			var result []vfilter.Row
			var all bool = false
			_, all = args.Get("all")

			if partitions, err := disk.Partitions(all); err == nil {
				for _, item := range partitions {
					extended_info := ExtendedFileSystemInfo{item}
					result = append(result, extended_info)
				}
			}

			return result
		},
		RowType: ExtendedFileSystemInfo{},
	}
}
