//go:build !windows
// +build !windows

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

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/psutils"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type PslistArgs struct {
	Pid int64 `vfilter:"optional,field=pid,doc=A process ID to list. If not provided list all processes."`
}

type PslistPlugin struct{}

func (self PslistPlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {

	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "pslist", args)()

		err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
		if err != nil {
			scope.Log("pslist: %s", err)
			return
		}

		arg := &PslistArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("pslist: %s", err.Error())
			return
		}

		// If the user asked for one process
		// just return that one.
		if arg.Pid != 0 {
			process_obj, err := psutils.GetProcess(ctx, int32(arg.Pid))
			if err == nil {
				select {
				case <-ctx.Done():
					return
				case output_chan <- process_obj:
					return
				}
			}
		}

		processes, err := psutils.ListProcesses(ctx)
		if err != nil {
			scope.Log("pslist: %s", err.Error())
			return
		}

		for _, item := range processes {
			select {
			case <-ctx.Done():
				return
			case output_chan <- item:
			}
		}
	}()

	return output_chan
}

func (self PslistPlugin) Info(
	scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "pslist",
		Metadata: vql_subsystem.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
		ArgType:  type_map.AddType(scope, &PslistArgs{}),
		Doc:      "List processes",
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&PslistPlugin{})
}
