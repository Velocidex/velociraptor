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
	"runtime"

	fqdn "github.com/Showmax/go-fqdn"
	"github.com/Velocidex/ordereddict"
	"github.com/shirou/gopsutil/host"

	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/vfilter"
)

type InfoStat struct {
	host.InfoStat
	Fqdn         string
	Architecture string
}

func getInfo(host *host.InfoStat) *ordereddict.Dict {
	return ordereddict.NewDict().
		Set("Hostname", host.Hostname).
		Set("Uptime", host.Uptime).
		Set("BootTime", host.BootTime).
		Set("Procs", host.Procs).
		Set("OS", host.OS).
		Set("Platform", host.Platform).
		Set("PlatformFamily", host.PlatformFamily).
		Set("PlatformVersion", host.PlatformVersion).
		Set("KernelVersion", host.KernelVersion).
		Set("VirtualizationSystem", host.VirtualizationSystem).
		Set("VirtualizationRole", host.VirtualizationRole).
		Set("HostID", host.HostID)
}

func init() {
	RegisterPlugin(
		vfilter.GenericListPlugin{
			PluginName: "info",
			Function: func(
				scope *vfilter.Scope,
				args *ordereddict.Dict) []vfilter.Row {
				var result []vfilter.Row

				err := CheckAccess(scope, acls.MACHINE_STATE)
				if err != nil {
					scope.Log("info: %s", err)
					return result
				}

				if info, err := host.Info(); err == nil {
					item := getInfo(info).
						Set("Fqdn", fqdn.Get()).
						Set("Architecture", runtime.GOARCH)
					result = append(result, item)
				}
				return result
			},
			Doc: "Get information about the running host.",
		})
}
