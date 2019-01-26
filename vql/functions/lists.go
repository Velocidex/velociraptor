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

	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type JoinFunction struct{}

func flatten(scope *vfilter.Scope, a vfilter.Any) []vfilter.Any {
	var result []vfilter.Any

	a_value := reflect.Indirect(reflect.ValueOf(a))
	a_type := a_value.Type()

	if a_type.Kind() == reflect.Slice {
		for i := 0; i < a_value.Len(); i++ {
			element := a_value.Index(i).Interface()
			flattened := flatten(scope, element)

			result = append(result, flattened...)
		}
		return result
	}

	members := scope.GetMembers(a)
	if len(members) > 0 {
		for _, item := range members {
			value, pres := scope.Associative(a, item)
			if pres {
				result = append(result, flatten(scope, value)...)
			}
		}

		return result
	}

	return []vfilter.Any{a}
}

func (self *JoinFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	return flatten(scope, args)
}

func (self JoinFunction) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name: "array",
		Doc:  "Create an array with all the args.",
	}
}

type FilterFunctionArgs struct {
	List  []string `vfilter:"required,field=list"`
	Regex []string `vfilter:"required,field=regex"`
}
type FilterFunction struct{}

func (self *FilterFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	arg := &FilterFunctionArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("filter: %s", err.Error())
		return &vfilter.Null{}
	}

	res := []*regexp.Regexp{}
	for _, re := range arg.Regex {
		r, err := regexp.Compile(re)
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

func (self FilterFunction) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name: "filter",
		Doc:  "Filters a strings array by regex.",
	}
}

func init() {
	vql_subsystem.RegisterFunction(&FilterFunction{})
	vql_subsystem.RegisterFunction(&JoinFunction{})
}
