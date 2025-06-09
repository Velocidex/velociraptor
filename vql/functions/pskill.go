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
package functions

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/psutils"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type PsKillFunctionArgs struct {
	Pid int64 `vfilter:"required,field=pid,doc=A pid to kill."`
}

type PsKillFunction struct{}

func (self *PsKillFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "pskill", args)()

	// Need high level of access to run this - basically the same as
	// shelling out to e.g. powershell.
	err := vql_subsystem.CheckAccess(scope, acls.EXECVE)
	if err != nil {
		scope.Log("pskill: %s", err)
		return vfilter.Null{}
	}

	arg := &PsKillFunctionArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("pskill: %v", err)
		return vfilter.Null{}
	}

	err = psutils.Kill(int32(arg.Pid))
	if err != nil {
		scope.Log("pskill: %v", err)
		return vfilter.Null{}
	}
	return arg.Pid
}

func (self PsKillFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "pskill",
		Doc:      "Kill the specified process.",
		ArgType:  type_map.AddType(scope, &PsKillFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.EXECVE).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&PsKillFunction{})
}
