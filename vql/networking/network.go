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
package networking

import (
	"context"
	"net"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type InterfacesPlugin struct {
}

func (self InterfacesPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "interfaces",
		Doc:      "List all active interfaces.",
		Metadata: vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	}
}

func (self InterfacesPlugin) Call(
	ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "interfaces", args)()

		err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
		if err != nil {
			scope.Log("interfaces: %s", err)
			return
		}

		interfaces, err := net.Interfaces()
		if err != nil {
			scope.Log("interfaces: failed to enumerate interfaces: %s", err)
			return
		}

		for _, iface := range interfaces {
			row := ordereddict.NewDict().
				Set("Name", iface.Name).
				Set("HardwareAddr", iface.HardwareAddr).
				Set("MTU", iface.MTU).
				Set("Index", iface.Index).
				Set("Flags", iface.Flags.String())

			// Enumerate some useful flags
			if (iface.Flags & net.FlagLoopback) == net.FlagLoopback {
				row.Set("Loopback", "Y")
			} else {
				row.Set("Loopback", "N")
			}

			if (iface.Flags & net.FlagPointToPoint) == net.FlagPointToPoint {
				row.Set("PointToPoint", "Y")
			} else {
				row.Set("PointToPoint", "N")
			}

			if (iface.Flags & net.FlagUp) == net.FlagUp {
				row.Set("Up", "Y")
			} else {
				row.Set("Up", "N")
			}

			// Add net.FlagRunning once we require go 1.20
			//			if (iface.Flags & net.FlagRunning) == net.FlagRunning {
			//				row.Set("Running", "Y")
			//			} else {
			//				row.Set("Running", "N")
			//			}

			row.Set("HardwareAddrString", iface.HardwareAddr.String())

			addrs, err := iface.Addrs()
			if err != nil {
				scope.Log("interfaces: Failed to get addresses for interface %s: %s",
					iface.Name, err)
				continue
			}
			row.Set("Addrs", addrs)

			addrList := []string{}
			for _, addr := range addrs {
				addrList = append(addrList, addr.String())
			}
			row.Set("AddrsString", addrList)

			addrs, err = iface.MulticastAddrs()
			if err != nil {
				scope.Log("interfaces: Failed to get multicast addresses for interface %s: %s",
					iface.Name, err)
			}
			row.Set("MulticastAddrs", addrs)

			addrList = []string{}
			for _, addr := range addrs {
				addrList = append(addrList, addr.String())
			}
			row.Set("MulticastAddrsString", addrList)

			output_chan <- row
		}
	}()

	return output_chan
}

func init() {
	vql_subsystem.RegisterPlugin(&InterfacesPlugin{})
}
