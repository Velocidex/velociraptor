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

package vql

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/vql/psutils"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type PslistArgs struct {
	Pid int64 `vfilter:"optional,field=pid,doc=A process ID to list. If not provided list all processes."`
}

func init() {
	RegisterPlugin(vfilter.GenericListPlugin{
		PluginName: "pslist",
		Metadata:   VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
		Function: func(
			ctx context.Context,
			scope vfilter.Scope,
			args *ordereddict.Dict) []vfilter.Row {
			var result []vfilter.Row

			err := CheckAccess(scope, acls.MACHINE_STATE)
			if err != nil {
				scope.Log("pslist: %s", err)
				return result
			}

			arg := &PslistArgs{}
			err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
			if err != nil {
				scope.Log("pslist: %s", err.Error())
				return result
			}

			// If the user asked for one process
			// just return that one.
			if arg.Pid != 0 {
				process_obj, err := psutils.GetProcess(ctx, int32(arg.Pid))
				if err == nil {
					result = append(result, process_obj)
				}
				return result
			}

			processes, err := psutils.ListProcesses(ctx)
			if err == nil {
				for _, item := range processes {
					result = append(result, item)
				}
			}
			return result
		},
		ArgType: &PslistArgs{},
		Doc:     "List processes",
	})
}
