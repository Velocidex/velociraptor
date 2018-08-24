package networking

import (
	"net"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

func init() {
	vql_subsystem.RegisterPlugin(
		&vfilter.GenericListPlugin{
			PluginName: "interfaces",
			Function: func(
				scope *vfilter.Scope,
				args *vfilter.Dict) []vfilter.Row {
				var result []vfilter.Row
				if interfaces, err := net.Interfaces(); err == nil {
					for _, item := range interfaces {
						local_item := item
						result = append(result, &local_item)
					}
				}

				return result
			},
			RowType: net.Interface{},
			Doc:     "List all active interfaces.",
		})
}
