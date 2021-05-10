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

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type LogFunctionArgs struct {
	Message string `vfilter:"required,field=message,doc=Message to log."`
}

type LogFunction struct{}

func (self *LogFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &LogFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("log: %s", err.Error())
		return false
	}

	last_log_str, ok := scope.GetContext("last_log")
	if ok {
		last_log, ok := last_log_str.(string)
		if ok && arg.Message == last_log {
			return true
		}
	}

	scope.Log("%v", arg.Message)
	scope.SetContext("last_log", arg.Message)

	return true
}

func (self LogFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "log",
		Doc:     "Log the message.",
		ArgType: type_map.AddType(scope, &LogFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&LogFunction{})
}
