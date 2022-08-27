// +build !windows

/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2022 Rapid7 Inc.

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

// This module is built on gopsutils but this is too slow and
// inefficient. Eventually we will remove it from the codebase.
package vql

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"github.com/shirou/gopsutil/v3/process"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type PslistArgs struct {
	Pid int64 `vfilter:"optional,field=pid,doc=A pid to list. If this is provided we are able to operate much faster by only opening a single process."`
}

func init() {
	RegisterPlugin(vfilter.GenericListPlugin{
		PluginName: "pslist",
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
				process_obj, err := process.NewProcess(int32(arg.Pid))
				if err == nil {
					result = append(result, process_obj)
				}
				return result
			}

			processes, err := process.Processes()
			if err == nil {
				for _, item := range processes {
					result = append(result, getProcessData(item))
				}
			}
			return result
		},
		ArgType: &PslistArgs{},
		Doc:     "List processes",
	})
}

// Only get a few fields from the process object otherwise we will
// spend too much time calling into virtual methods.
func getProcessData(process *process.Process) *ordereddict.Dict {
	result := ordereddict.NewDict().Set("Pid", process.Pid)

	name, _ := process.Name()
	result.Set("Name", name)

	ppid, _ := process.Ppid()
	result.Set("Ppid", ppid)

	// Make it compatible with the Windows pslist()
	cmdline, _ := process.Cmdline()
	result.Set("CommandLine", cmdline)

	create_time, _ := process.CreateTime()
	result.Set("CreateTime", create_time)

	exe, _ := process.Exe()
	result.Set("Exe", exe)

	cwd, _ := process.Cwd()
	result.Set("Cwd", cwd)

	user, _ := process.Username()
	result.Set("Username", user)

	memory_info, _ := process.MemoryInfo()
	result.Set("MemoryInfo", memory_info)

	return result
}
