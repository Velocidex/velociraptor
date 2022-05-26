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
	"context"
	"os"
	"runtime"

	fqdn "github.com/Showmax/go-fqdn"
	"github.com/Velocidex/ordereddict"
	"github.com/shirou/gopsutil/v3/host"

	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

func getInfo(host *host.InfoStat) *ordereddict.Dict {
	me, _ := os.Executable()
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
		Set("CompilerVersion", runtime.Version()).
		Set("HostID", host.HostID).
		Set("Exe", me).
		Set("IsAdmin", IsAdmin())
}

func init() {
	RegisterPlugin(
		vfilter.GenericListPlugin{
			PluginName: "info",
			Function: func(
				ctx context.Context,
				scope vfilter.Scope,
				args *ordereddict.Dict) []vfilter.Row {
				var result []vfilter.Row

				err := CheckAccess(scope, acls.MACHINE_STATE)
				if err != nil {
					scope.Log("info: %s", err)
					return result
				}

				arg := &vfilter.Empty{}
				err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
				if err != nil {
					scope.Log("info: %s", err.Error())
					return result
				}

				// It turns out that host.Info() is
				// actually rather slow so we cache it
				// in the scope cache.
				info, ok := CacheGet(scope, "__info").(*host.InfoStat)
				if !ok {
					info, err = host.Info()
					if err != nil {
						scope.Log("info: %s", err)
						return result
					}
					CacheSet(scope, "__info", info)
				}

				item := getInfo(info).
					Set("Fqdn", fqdn.Get()).
					Set("Architecture", runtime.GOARCH)
				result = append(result, item)

				return result
			},
			Doc: "Get information about the running host.",
		})
}
