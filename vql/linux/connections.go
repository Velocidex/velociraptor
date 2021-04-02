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
	"fmt"
	"github.com/Velocidex/ordereddict"
	"github.com/shirou/gopsutil/net"
	"www.velocidex.com/golang/velociraptor/acls"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	CommonName = map[string]string{
		"11":  "UNIX_SOCK|SOCK_STREAM",
		"12":  "UNIX_SOCK|SOCK_DGRAM",
		"15":  "UNIX_SOCK|SOCK_SEQPACKET",
		"21":  "tcp",
		"22":  "udp",
		"101": "tcp6",
		"102": "udp6",
	}
)

type ConnectionStatEnhanced struct {
	Fd         uint32   `json:"fd"`
	Family     uint32   `json:"family"`
	Type       uint32   `json:"type"`
	CommonName string   `json:"commonname"`
	Laddr      net.Addr `json:"localaddr"`
	Raddr      net.Addr `json:"remoteaddr"`
	Status     string   `json:"status"`
	Uids       []int32  `json:"uids"`
	Pid        int32    `json:"pid"`
}

func get_conns(
	scope vfilter.Scope,
	args *ordereddict.Dict) []vfilter.Row {
	var result []vfilter.Row

	err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
	if err != nil {
		scope.Log("connections: %s", err)
		return result
	}

	if cons, err := net.Connections("all"); err == nil {
		for _, item := range cons {
			match_string := fmt.Sprintf("%d%d", item.Family, item.Type)
			common_name, ok := CommonName[match_string]
			if !ok {
				scope.Log("Unknown Family/Type: %d %d", item.Family, item.Type)
				common_name = "UNKNOWN"
			}
			conn := ConnectionStatEnhanced{item.Fd, item.Family, item.Type, common_name,
				item.Laddr, item.Raddr, item.Status, item.Uids, item.Pid}
			result = append(result, conn)
		}
	}
	return result
}

func init() {
	vql_subsystem.RegisterPlugin(
		&vfilter.GenericListPlugin{
			PluginName: "connections",
			Function:   get_conns,
			Doc:        "List all active connections",
		})
	vql_subsystem.RegisterPlugin(
		&vfilter.GenericListPlugin{
			PluginName: "netstat",
			Function:   get_conns,
			Doc:        "List all active connections (alias to connections)",
		})
}
