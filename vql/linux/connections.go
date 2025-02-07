/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

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
	"context"
	"fmt"
	"syscall"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/psutils"
	"www.velocidex.com/golang/vfilter"
)

func makeDict(in psutils.ConnectionStat) *ordereddict.Dict {
	var family, conn_type string

	switch in.Family {
	case syscall.AF_INET:
		family = "AF_INET"

	case syscall.AF_INET6:
		family = "AF_INET6"

	case syscall.AF_UNIX:
		family = "AF_UNIX"

	default:
		family = fmt.Sprintf("%d", in.Family)
	}

	switch in.Type {
	case syscall.SOCK_STREAM:
		conn_type = "TCP"

	case syscall.SOCK_DGRAM:
		conn_type = "UCP"

	default:
		conn_type = fmt.Sprintf("%v", in.Type)
	}

	return ordereddict.NewDict().SetCaseInsensitive().
		Set("FD", in.Fd).
		Set("Family", family).
		Set("Type", conn_type).
		Set("Laddr", ordereddict.NewDict().SetCaseInsensitive().
			Set("ip", in.Laddr.IP).
			Set("port", in.Laddr.Port)).
		Set("Raddr", ordereddict.NewDict().SetCaseInsensitive().
			Set("ip", in.Raddr.IP).
			Set("port", in.Raddr.Port)).
		Set("Status", in.Status).
		Set("Pid", in.Pid).
		Set("Uids", in.Uids)
}

func init() {
	vql_subsystem.RegisterPlugin(
		&vfilter.GenericListPlugin{
			PluginName: "connections",
			Metadata:   vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
			Function: func(
				ctx context.Context,
				scope vfilter.Scope,
				args *ordereddict.Dict) []vfilter.Row {
				var result []vfilter.Row

				err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
				if err != nil {
					scope.Log("connections: %s", err)
					return result
				}

				cons, err := psutils.ConnectionsWithContext(ctx, "all")
				if err == nil {
					for _, item := range cons {
						result = append(result, makeDict(item))
					}
				}
				return result
			},
			Doc: "List all active connections",
		})
}
