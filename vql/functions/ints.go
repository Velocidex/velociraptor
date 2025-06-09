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
	"fmt"
	"strconv"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type IntArgs struct {
	Int vfilter.Any `vfilter:"optional,field=int,doc=The integer to round"`
}

type IntFunction struct{}

func (self *IntFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "int", args)()

	arg := &IntArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("int: %s", err.Error())
		return false
	}

	switch t := arg.Int.(type) {
	case string:
		result, _ := strconv.ParseInt(t, 0, 64)
		return result

	case float32:
		return int64(t)

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

	case int32:
		return int64(t)

	}

	return 0
}

func (self IntFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "int",
		Doc:     "Truncate to an integer.",
		ArgType: type_map.AddType(scope, &IntArgs{}),
	}
}

type StrFunctionArgs struct {
	Str vfilter.Any `vfilter:"required,field=str,doc=The string to normalize"`
}

type StrFunction struct{}

func (self *StrFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "str", args)()

	arg := &StrFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("str: %s", err.Error())
		return false
	}

	switch t := arg.Str.(type) {
	case string:
		return string(t)

	case []byte:
		return string(t)

	default:
		return fmt.Sprintf("%v", t)
	}
}

func (self StrFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "str",
		Doc:     "Normalize a String.",
		ArgType: type_map.AddType(scope, &StrFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&IntFunction{})
	vql_subsystem.RegisterFunction(&StrFunction{})
}
