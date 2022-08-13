/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2022 Rapid7 Inc.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package filesystem

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"github.com/shirou/gopsutil/v3/disk"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
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
	All bool `vfilter:"optional,field=all,doc=If specified list all Partitions"`
}

func init() {
	vql_subsystem.RegisterPlugin(
		&vfilter.GenericListPlugin{
			PluginName: "partitions",
			Function: func(
				ctx context.Context,
				scope vfilter.Scope,
				args *ordereddict.Dict) []vfilter.Row {
				var result []vfilter.Row

				arg := &PartitionsArgs{}
				err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
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
			ArgType: &PartitionsArgs{},
			Doc:     "List all partititions",
		})
}
