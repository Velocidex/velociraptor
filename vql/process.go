// +build !windows

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

// This module is built on gopsutils but this is too slow and
// inefficient. Eventually we will remove it from the codebase.
package vql

import (
	"github.com/Velocidex/ordereddict"
	"github.com/shirou/gopsutil/process"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

// Block potentially dangerous methods.
var _BlockedMembers = []string{"Terminate", "Kill", "Suspend", "Resume"}

type _ProcessFieldImpl struct{}

func (self _ProcessFieldImpl) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, b_ok := b.(string)
	switch a.(type) {
	case process.Process, *process.Process:
		return b_ok
	}
	return false
}

func (self _ProcessFieldImpl) Associative(
	scope *vfilter.Scope, a vfilter.Any, b vfilter.Any) (vfilter.Any, bool) {
	field := b.(string)

	if utils.InString(_BlockedMembers, field) {
		return false, true
	}

	res, pres := vfilter.DefaultAssociative{}.Associative(scope, a, b)
	return res, pres
}

func (self _ProcessFieldImpl) GetMembers(scope *vfilter.Scope, a vfilter.Any) []string {
	var result []string
	for _, item := range (vfilter.DefaultAssociative{}).GetMembers(scope, a) {
		if !utils.InString(_BlockedMembers, item) {
			result = append(result, item)
		}
	}

	return result
}

type PslistArgs struct {
	Pid int64 `vfilter:"optional,field=pid,doc=A pid to list. If this is provided we are able to operate much faster by only opening a single process."`
}

func init() {
	RegisterProtocol(&_ProcessFieldImpl{})
	RegisterPlugin(vfilter.GenericListPlugin{
		PluginName: "pslist",
		Function: func(
			scope *vfilter.Scope,
			args *ordereddict.Dict) []vfilter.Row {
			var result []vfilter.Row

			err := CheckAccess(scope, acls.MACHINE_STATE)
			if err != nil {
				scope.Log("pslist: %s", err)
				return result
			}

			arg := &PslistArgs{}
			err = vfilter.ExtractArgs(scope, args, arg)
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
					result = append(result, item)
				}
			}
			return result
		},
		ArgType: &PslistArgs{},
		Doc:     "List processes",
	})
}
