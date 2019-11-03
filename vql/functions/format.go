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
	"fmt"
	"reflect"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type FormatArgs struct {
	Format string      `vfilter:"required,field=format,doc=Format string to use"`
	Args   vfilter.Any `vfilter:"optional,field=args,doc=An array of elements to apply into the format string."`
}

type FormatFunction struct{}

func (self *FormatFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &FormatArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("format: %s", err.Error())
		return false
	}

	var format_args []interface{}
	slice := reflect.ValueOf(arg.Args)

	// A slice of strings.
	if slice.Type().Kind() != reflect.Slice {
		format_args = append(format_args, arg.Args)
	} else {
		for i := 0; i < slice.Len(); i++ {
			value := slice.Index(i).Interface()
			format_args = append(format_args, value)
		}
	}

	return fmt.Sprintf(arg.Format, format_args...)
}

func (self FormatFunction) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "format",
		Doc:     "Format one or more items according to a format string.",
		ArgType: type_map.AddType(scope, &FormatArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&FormatFunction{})
}
