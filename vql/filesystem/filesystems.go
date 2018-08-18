package filesystem

import (
	"github.com/shirou/gopsutil/disk"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
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

/*
func (self ExtendedFileSystemInfo) SerialNumber() string {
	return disk.GetDiskSerialNumber(self.Partition.Device)
}
*/

type PartitionsArgs struct {
	All bool `vfilter:"optional,field=all"`
}

func init() {
	vql_subsystem.RegisterPlugin(
		&vfilter.GenericListPlugin{
			PluginName: "partitions",
			Function: func(
				scope *vfilter.Scope,
				args *vfilter.Dict) []vfilter.Row {
				var result []vfilter.Row

				arg := &PartitionsArgs{}
				err := vfilter.ExtractArgs(scope, args, arg)
				if err != nil {
					scope.Log("%s: %s", "partitions", err.Error())
					return result
				}

				if partitions, err := disk.Partitions(arg.All); err == nil {
					for _, item := range partitions {
						extended_info := ExtendedFileSystemInfo{item}
						result = append(result, extended_info)
					}
				}

				return result
			},
			RowType: ExtendedFileSystemInfo{},
			ArgType: &PartitionsArgs{},
			Doc:     "List all partititions",
		})
}
