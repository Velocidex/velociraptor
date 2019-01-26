/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

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
			Doc:     "Get information about the running host.",
		})
}
