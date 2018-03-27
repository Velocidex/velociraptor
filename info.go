package velociraptor

import (
	"www.velocidex.com/golang/vfilter"
	"github.com/shirou/gopsutil/host"
)

func MakeInfoPlugin() vfilter.GenericListPlugin {
	return vfilter.GenericListPlugin{
		PluginName: "info",
		Function: func(args vfilter.Dict) []vfilter.Row {
			var result []vfilter.Row
			if info, err := host.Info(); err ==  nil {
				result = append(result, info)
			}

			return result
		},
		RowType: host.InfoStat{},
	}
}
