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
package functions

import (
	"context"

	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type IntArgs struct {
	Int vfilter.Any `vfilter:"optional,field=int"`
}

type IntFunction struct{}

func (self *IntFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	arg := &IntArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("int: %s", err.Error())
		return false
	}

	switch t := arg.Int.(type) {
	case float64:
		return int64(t)
	case int:
		return int64(t)
	case int64:
		return int64(t)
	case uint64:
		return int64(t)
	case uint32:
		return uint64(t)
	}

	return 0
}

func (self IntFunction) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "int",
		Doc:     "Truncate to an integer.",
		ArgType: type_map.AddType(scope, &IntArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&IntFunction{})
}
