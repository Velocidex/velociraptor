package vql

import (
	"github.com/shirou/gopsutil/net"
	"www.velocidex.com/golang/vfilter"
)

func MakeInterfacesPlugin() vfilter.GenericListPlugin {
	return vfilter.GenericListPlugin{
		PluginName: "interfaces",
		Function: func(
			scope *vfilter.Scope,
			args *vfilter.Dict) []vfilter.Row {
			var result []vfilter.Row
			if interfaces, err := net.Interfaces(); err == nil {
				for _, item := range interfaces {
					result = append(result, item)
				}
			}

			return result
		},
		RowType: net.InterfaceStat{},
	}

}
