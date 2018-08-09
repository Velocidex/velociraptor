package vql

import (
	"github.com/Showmax/go-fqdn"
	"github.com/shirou/gopsutil/host"
	"runtime"
	"www.velocidex.com/golang/vfilter"
)

type InfoStat struct {
	host.InfoStat
	Fqdn         string
	Architecture string
}

func init() {
	exportedPlugins = append(exportedPlugins,
		vfilter.GenericListPlugin{
			PluginName: "info",
			Function: func(
				scope *vfilter.Scope,
				args *vfilter.Dict) []vfilter.Row {
				var result []vfilter.Row
				if info, err := host.Info(); err == nil {
					item := InfoStat{*info,
						fqdn.Get(),
						runtime.GOARCH}
					result = append(result, item)
				}

				return result
			},
			RowType: InfoStat{},
		})
}
