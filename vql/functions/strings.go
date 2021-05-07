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
	"strings"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type StripArgs struct {
	String string `vfilter:"required,field=string,doc=The string to strip"`
	Prefix string `vfilter:"optional,field=prefix,doc=The prefix to strip"`
}

type StripFunction struct{}

func (self *StripFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &StripArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("strip: %s", err.Error())
		return false
	}
	if arg.Prefix == "" {
		return strings.TrimSpace(arg.String)
	}
	return strings.TrimPrefix(arg.String, arg.Prefix)
}

func (self StripFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "strip",
		Doc:     "Strip a prefix from a string.",
		ArgType: type_map.AddType(scope, &StripArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&StripFunction{})
}
