package vql

import (
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/net"
	"www.velocidex.com/golang/vfilter"
)

func MakeUsersPlugin() vfilter.GenericListPlugin {
	return vfilter.GenericListPlugin{
		PluginName: "users",
		Function: func(args *vfilter.Dict) []vfilter.Row {
			var result []vfilter.Row
			if users, err := host.Users(); err == nil {
				for _, item := range users {
					result = append(result, item)
				}
			}
			return result
		},
		RowType: host.UserStat{},
	}
}

func MakeConnectionsPlugin() vfilter.GenericListPlugin {
	return vfilter.GenericListPlugin{
		PluginName: "connections",
		Function: func(args *vfilter.Dict) []vfilter.Row {
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
