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
	"strings"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type StripArgs struct {
	String string `vfilter:"required,field=string,doc=The string to strip"`
	Prefix string `vfilter:"optional,field=prefix,doc=The prefix to strip"`
	Suffix string `vfilter:"optional,field=suffix,doc=The suffix to strip"`
}

type StripFunction struct{}

func (self *StripFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "strip", args)()

	arg := &StripArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("strip: %s", err.Error())
		return false
	}
	if arg.Prefix == "" && arg.Suffix == "" {
		return strings.TrimSpace(arg.String)
	}

	s := arg.String

	if arg.Prefix != "" {
		s = strings.TrimPrefix(s, arg.Prefix)
	}

	if arg.Suffix != "" {
		s = strings.TrimSuffix(s, arg.Suffix)
	}

	return s
}

func (self StripFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "strip",
		Doc:     "Strip a prefix or suffix from a string.",
		ArgType: type_map.AddType(scope, &StripArgs{}),
	}
}

type SubStrFunction struct{}

type SubStrArgs struct {
	Str   string `vfilter:"required,field=str,doc=The string to shorten"`
	Start int    `vfilter:"optional,field=start,doc=Beginning index of substring"`
	End   int    `vfilter:"optional,field=end,doc=End index of substring"`
}

func (self *SubStrFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "substr", args)()

	arg := &SubStrArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("substr: %s", err.Error())
		return nil
	}

	if arg.Start < 0 || arg.End < 0 {
		scope.Log("substr: Start and End values must be greater than 0")
		return nil
	}

	if arg.Start == 0 && arg.End == 0 {
		return arg.Str
	} else if arg.Start != 0 && arg.End == 0 {
		arg.End = len(arg.Str)
	} else if arg.End < arg.Start {
		scope.Log("substr: End must be greater than start!")
		return nil
	}

	counter, start_index := 0, 0
	for i := range arg.Str {
		if counter == arg.Start {
			start_index = i
		}
		if counter == arg.End {
			return arg.Str[start_index:i]
		}
		counter++
	}
	return arg.Str[start_index:]
}

func (self *SubStrFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "substr",
		Doc:     "Create a substring from a string",
		ArgType: type_map.AddType(scope, &SubStrArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&StripFunction{})
	vql_subsystem.RegisterFunction(&SubStrFunction{})
}
