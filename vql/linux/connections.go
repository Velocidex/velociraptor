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
package linux

import (
	"github.com/Velocidex/ordereddict"
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
				args *ordereddict.Dict) []vfilter.Row {
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
