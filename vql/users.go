package vql

import (
	"github.com/shirou/gopsutil/net"
	"www.velocidex.com/golang/vfilter"
)

func MakeConnectionsPlugin() vfilter.GenericListPlugin {
	return vfilter.GenericListPlugin{
		PluginName: "connections",
		Function: func(
			scope *vfilter.Scope,
			args *vfilter.Dict) []vfilter.Row {
			var result []vfilter.Row
			if cons, err := net.Connections("all"); err == nil {
				for _, item := range cons {
					result = append(result, item)
				}
			}
			return result
		},
		RowType: net.ConnectionStat{},
	}
}
