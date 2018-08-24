package vql

import (
	"github.com/shirou/gopsutil/net"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

func init() {
	vql_subsystem.RegisterPlugin(
		&vfilter.GenericListPlugin{
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
			Doc:     "List all active connections",
			RowType: net.ConnectionStat{},
		})
}
