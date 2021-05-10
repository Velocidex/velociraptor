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
package networking

import (
	"context"
	"net"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

func init() {
	vql_subsystem.RegisterPlugin(
		&vfilter.GenericListPlugin{
			PluginName: "interfaces",
			Function: func(
				ctx context.Context,
				scope vfilter.Scope,
				args *ordereddict.Dict) []vfilter.Row {
				var result []vfilter.Row

				err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
				if err != nil {
					scope.Log("interfaces: %s", err)
					return result
				}

				if interfaces, err := net.Interfaces(); err == nil {
					for _, item := range interfaces {
						local_item := item
						result = append(result, &local_item)
					}
				}

				return result
			},
			Doc: "List all active interfaces.",
		})
}
