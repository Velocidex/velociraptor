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
package common

import (
	"context"
	"os"
	"strings"

	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type EnvPluginArgs struct {
	Vars []string `vfilter:"optional,field=vars,doc=Extract these variables from the environment and return them one per row"`
}

type EnvFunctionArgs struct {
	Var string `vfilter:"required,field=var,doc=Extract the var from the environment."`
}

type EnvFunction struct{}

func (self *EnvFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	arg := &EnvFunctionArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("environ: %s", err.Error())
		return false
	}

	return os.Getenv(arg.Var)
}

func (self *EnvFunction) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "environ",
		Doc:     "Get an environment variable.",
		ArgType: type_map.AddType(scope, &EnvFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&EnvFunction{})
	vql_subsystem.RegisterPlugin(
		vfilter.GenericListPlugin{
			PluginName: "environ",
			Function: func(
				scope *vfilter.Scope,
				args *vfilter.Dict) []vfilter.Row {
				var result []vfilter.Row

				arg := &EnvPluginArgs{}
				err := vfilter.ExtractArgs(scope, args, arg)
				if err != nil {
					scope.Log("%s: %s", "environ", err.Error())
					return result
				}

				row := vfilter.NewDict().
					SetDefault(&vfilter.Null{}).
					SetCaseInsensitive()
				if len(arg.Vars) == 0 {
					for _, env_var := range os.Environ() {
						parts := strings.SplitN(env_var, "=", 2)
						if len(parts) > 1 {
							row.Set(parts[0], parts[1])
						}
					}
				} else {
					for _, env_var := range arg.Vars {
						value, pres := os.LookupEnv(env_var)
						if pres {
							row.Set(env_var, value)
						}
					}
				}

				if row.Len() > 0 {
					result = append(result, row)
				}
				return result
			},
			ArgType: &EnvPluginArgs{},
		})
}
