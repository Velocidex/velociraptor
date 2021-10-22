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
	"reflect"
	"regexp"
	"strings"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type ArrayFunction struct{}

func flatten(ctx context.Context, scope vfilter.Scope, a vfilter.Any, depth int) []vfilter.Any {
	var result []vfilter.Any

	if depth > 4 {
		return result
	}

	switch t := a.(type) {
	case types.LazyExpr:
		a = t.Reduce(ctx)

	case types.StoredQuery:
		for row := range t.Eval(ctx, scope) {
			// Special case a single column means the
			// value is taken directly.
			members := scope.GetMembers(row)
			if len(members) == 1 {
				row, _ = scope.Associative(row, members[0])
			}
			flattened := flatten(ctx, scope, row, depth+1)
			result = append(result, flattened...)
		}
		return result
	}

	a_value := reflect.Indirect(reflect.ValueOf(a))
	a_type := a_value.Type()

	if a_type.Kind() == reflect.Slice {
		for i := 0; i < a_value.Len(); i++ {
			element := a_value.Index(i).Interface()
			flattened := flatten(ctx, scope, element, depth+1)

			result = append(result, flattened...)
		}
		return result
	}

	members := scope.GetMembers(a)
	if len(members) > 0 {
		for _, item := range members {
			value, pres := scope.Associative(a, item)
			if pres {
				result = append(result, flatten(
					ctx, scope, value, depth+1)...)
			}
		}

		return result
	}

	return []vfilter.Any{a}
}

func (self *ArrayFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	return flatten(ctx, scope, args, 0)
}

func (self ArrayFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name: "array",
		Doc:  "Create an array with all the args.",
	}
}

type JoinFunctionArgs struct {
	Array []string `vfilter:"required,field=array,doc=The array to join"`
	Sep   string   `vfilter:"optional,field=sep,doc=The separator"`
}

type JoinFunction struct{}

func (self *JoinFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

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
	List  []string `vfilter:"required,field=list,doc=A list of items to filter"`
	Regex []string `vfilter:"required,field=regex,doc=A regex to test each item"`
}
type FilterFunction struct{}

func (self *FilterFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &FilterFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("filter: %s", err.Error())
		return &vfilter.Null{}
	}

	res := []*regexp.Regexp{}
	for _, re := range arg.Regex {
		r, err := regexp.Compile("(?i)" + re)
		if err != nil {
			scope.Log("filter: Unable to compile regex %s", re)
			return false
		}
		res = append(res, r)
	}

	result := []string{}
	for _, item := range arg.List {
		for _, regex := range res {
			if regex.MatchString(item) {
				result = append(result, item)
			}
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
	List vfilter.Any `vfilter:"required,field=list,doc=A list of items too filter"`
}
type LenFunction struct{}

func (self *LenFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &LenFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("len: %s", err.Error())
		return &vfilter.Null{}
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
	arg := &SliceFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("len: %s", err.Error())
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
