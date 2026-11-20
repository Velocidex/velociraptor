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
package tools

import (
	"context"
	"os"
	"runtime"
	"time"

	fqdn "github.com/Showmax/go-fqdn"
	"github.com/Velocidex/ordereddict"

	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/psutils"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

var (
	start_time = time.Now()
)

func GetInfo(host *psutils.InfoStat) *ordereddict.Dict {
	me, _ := os.Executable()
	cwd, _ := os.Getwd()

	zone, tz_offset := time.Now().Local().Zone()

	return ordereddict.NewDict().
		Set("Hostname", host.Hostname).
		Set("Uptime", host.Uptime).
		Set("BootTime", host.BootTime).
		Set("OS", host.OS).
		Set("Platform", host.Platform).
		Set("PlatformFamily", host.PlatformFamily).
		Set("PlatformVersion", host.PlatformVersion).
		Set("KernelVersion", host.KernelVersion).
		Set("VirtualizationSystem", host.VirtualizationSystem).
		Set("VirtualizationRole", host.VirtualizationRole).
		Set("CompilerVersion", runtime.Version()).
		Set("HostID", psutils.HostID()).
		Set("Exe", me).
		Set("CWD", cwd).
		Set("IsAdmin", vql_subsystem.IsAdmin()).
		Set("ClientStart", start_time).
		Set("LocalTZ", zone).
		Set("LocalTZOffset", tz_offset)

}

func info(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &vfilter.Empty{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("info: %s", err.Error())
		return vfilter.Null{}
	}

	// It turns out that host.Info() is
	// actually rather slow so we cache it
	// in the scope cache.
	info, ok := vql_subsystem.CacheGet(scope, "__info").(*psutils.InfoStat)
	if !ok {
		info, err = psutils.InfoWithContext(ctx)
		if err != nil {
			scope.Log("info: %s", err)
			return vfilter.Null{}
		}
		vql_subsystem.CacheSet(scope, "__info", info)
	}

	return GetInfo(info).
		Set("Fqdn", fqdn.Get()).
		Set("Architecture", utils.GetArch())
}

type InfoFunction struct{}

func (self InfoFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
	if err != nil {
		scope.Log("info: %s", err)
		return vfilter.Null{}
	}

	return info(ctx, scope, args)
}

func (self InfoFunction) Info(
	scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name: "info",
		Metadata: vql_subsystem.VQLMetadata().
			Permissions(acls.MACHINE_STATE).Build(),
		Doc: "Get information about the running host. This is the function version of the info() plugin",
	}
}

type InfoPlugin struct{}

func (self InfoPlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "info", args)()

		err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
		if err != nil {
			scope.Log("info: %s", err)
			return
		}

		res := info(ctx, scope, args)
		if utils.IsNil(res) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case output_chan <- res:
		}
	}()

	return output_chan
}

func (self InfoPlugin) Info(
	scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "info",
		Metadata: vql_subsystem.VQLMetadata().
			ExecutionContext(vql_subsystem.MasterExecutionContext).
			Permissions(acls.MACHINE_STATE).Build(),
		Doc: "Get information about the running host.",
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&InfoPlugin{})
	vql_subsystem.RegisterFunction(&InfoFunction{})
}
