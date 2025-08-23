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
	"reflect"
	"strings"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type ArrayFunctionArgs struct {
	Value vfilter.Any `vfilter:"optional,field=_,doc=If specified we initialize the array from this parameter."`
}

type ArrayFunction struct{}

func (self *ArrayFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	defer vql_subsystem.RegisterMonitor(ctx, "array", args)()

	result := []vfilter.Any{}

	value, pres := args.Get("_")
	if pres {
		value = vql_subsystem.Materialize(ctx, scope, value)

		a_value := reflect.Indirect(reflect.ValueOf(value))
		a_type := a_value.Type()
		if a_type.Kind() == reflect.Slice {
			for i := 0; i < a_value.Len(); i++ {
				result = append(result, a_value.Index(i).Interface())
			}
		} else {
			result = append(result, value)
		}
	}

	for _, i := range args.Items() {
		if i.Key == "_" {
			continue
		}

		value = vql_subsystem.Materialize(ctx, scope, i.Value)
		result = append(result, value)
	}

	return result
}

func (self ArrayFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:         "array",
		Doc:          "Create an array with all the args.",
		FreeFormArgs: true,
	}
}

type JoinFunctionArgs struct {
	Array []string `vfilter:"required,field=array,doc=The array to join"`
	Sep   string   `vfilter:"optional,field=sep,doc=The separator. Defaults to an empty string if not explicitly set"`
}

type JoinFunction struct{}

func (self *JoinFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	defer vql_subsystem.RegisterMonitor(ctx, "join", args)()
	arg := &JoinFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("join: %s", err.Error())
		return false
	}

	return strings.Join(arg.Array, arg.Sep)
}

func (self JoinFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "join",
		Doc:     "Join all the args on a separator.",
		ArgType: type_map.AddType(scope, &JoinFunctionArgs{}),
	}
}

type FilterFunctionArgs struct {
	List      []vfilter.Any   `vfilter:"required,field=list,doc=A list of items to filter"`
	Regex     string          `vfilter:"optional,field=regex,doc=A regex to test each item"`
	Condition *vfilter.Lambda `vfilter:"optional,field=condition,doc=A VQL lambda to use to filter elements"`
}
type FilterFunction struct{}

func (self *FilterFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	defer vql_subsystem.RegisterMonitor(ctx, "filter", args)()
	arg := &FilterFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("filter: %s", err.Error())
		return &vfilter.Null{}
	}

	if arg.Condition != nil {
		if arg.Regex != "" {
			scope.Log("ERROR:filter: Both regex and condition are specified - Will only use the condition and ignore regex!")
		}

		result := []types.Any{}
		for _, item := range arg.List {
			if scope.Bool(arg.Condition.Reduce(ctx, scope, []vfilter.Any{item})) {
				result = append(result, item)
			}
		}
		return result
	}

	result := []types.Any{}
	for _, item := range arg.List {
		if scope.Match(arg.Regex, item) {
			result = append(result, item)
		}
	}
	return result
}

func (self FilterFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "filter",
		Doc:     "Filters a strings array by regex.",
		ArgType: type_map.AddType(scope, &FilterFunctionArgs{}),
	}
}

type LenFunctionArgs struct {
	List vfilter.Any `vfilter:"required,field=list,doc=A list of items to filter"`
}
type LenFunction struct{}

func (self *LenFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	defer vql_subsystem.RegisterMonitor(ctx, "len", args)()
	arg := &LenFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("len: %s", err.Error())
		return &vfilter.Null{}
	}

	switch t := arg.List.(type) {
	case types.LazyExpr:
		arg.List = t.Reduce(ctx)

	case types.Materializer:
		arg.List = t.Materialize(ctx, scope)
	}

	slice := reflect.ValueOf(arg.List)
	// A slice of strings. Only the following are supported
	// https://golang.org/pkg/reflect/#Value.Len
	if slice.Type().Kind() == reflect.Slice ||
		slice.Type().Kind() == reflect.Map ||
		slice.Type().Kind() == reflect.Array ||
		slice.Type().Kind() == reflect.String {
		return slice.Len()
	}

	dict, ok := arg.List.(*ordereddict.Dict)
	if ok {
		return dict.Len()
	}

	return 0
}

func (self LenFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "len",
		Doc:     "Returns the length of an object.",
		ArgType: type_map.AddType(scope, &LenFunctionArgs{}),
	}
}

type SliceFunctionArgs struct {
	List  vfilter.Any `vfilter:"required,field=list,doc=A list of items to slice"`
	Start uint64      `vfilter:"required,field=start,doc=Start index (0 based)"`
	End   uint64      `vfilter:"required,field=end,doc=End index (0 based)"`
}
type SliceFunction struct{}

func (self *SliceFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "slice", args)()

	arg := &SliceFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("slice: %s", err.Error())
		return &vfilter.Null{}
	}

	slice := reflect.ValueOf(arg.List)
	// A slice of strings. Only the following are supported
	// https://golang.org/pkg/reflect/#Value.Len
	if slice.Type().Kind() == reflect.Slice ||
		slice.Type().Kind() == reflect.Map ||
		slice.Type().Kind() == reflect.Array ||
		slice.Type().Kind() == reflect.String {

		if arg.End > uint64(slice.Len()) {
			arg.End = uint64(slice.Len())
		}

		if arg.Start > arg.End {
			arg.Start = arg.End
		}

		result := make([]interface{}, 0, arg.End-arg.Start)
		for i := arg.Start; i < arg.End; i++ {
			result = append(result, slice.Index(int(i)).Interface())
		}

		return result
	}

	dict, ok := arg.List.(*ordereddict.Dict)
	if ok {
		keys := dict.Keys()
		if arg.End > uint64(len(keys)) {
			arg.End = uint64(len(keys))
		}

		if arg.Start > arg.End {
			arg.Start = arg.End
		}

		result := make([]interface{}, 0, arg.End-arg.Start)
		for i := arg.Start; i < arg.End; i++ {
			result = append(result, keys[int(i)])
		}
		return result
	}

	return []vfilter.Any{}
}

func (self SliceFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "slice",
		Doc:     "Slice an array.",
		ArgType: type_map.AddType(scope, &SliceFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&SliceFunction{})
	vql_subsystem.RegisterFunction(&FilterFunction{})
	vql_subsystem.RegisterFunction(&ArrayFunction{})
	vql_subsystem.RegisterFunction(&JoinFunction{})
	vql_subsystem.RegisterFunction(&LenFunction{})
}
