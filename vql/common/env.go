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
package common

import (
	"context"
	"os"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

var (
	// Prevent VQL from having access to these critical env variables.
	ShadowedEnv = []string{
		constants.VELOCIRAPTOR_CONFIG,
		constants.VELOCIRAPTOR_LITERAL_CONFIG,
		constants.VELOCIRAPTOR_API_CONFIG,
	}
)

type EnvPluginArgs struct {
	Vars []string `vfilter:"optional,field=vars,doc=Extract these variables from the environment and return them one per row"`
}

type EnvFunctionArgs struct {
	Var string `vfilter:"required,field=var,doc=Extract the var from the environment."`
}

type EnvFunction struct{}

func (self *EnvFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &EnvFunctionArgs{}

	err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
	if err != nil {
		scope.Log("environ: %s", err)
		return false
	}

	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("environ: %s", err.Error())
		return false
	}

	if utils.InString(ShadowedEnv, arg.Var) {
		scope.Log("environ: access to env var %s is denied", arg.Var)
		return false
	}

	return os.Getenv(arg.Var)
}

func (self *EnvFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "environ",
		Doc:      "Get an environment variable.",
		ArgType:  type_map.AddType(scope, &EnvFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&EnvFunction{})
	vql_subsystem.RegisterPlugin(
		vfilter.GenericListPlugin{
			PluginName: "environ",
			Function: func(
				ctx context.Context,
				scope vfilter.Scope,
				args *ordereddict.Dict) []vfilter.Row {
				var result []vfilter.Row

				err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
				if err != nil {
					scope.Log("environ: %s", err)
					return result
				}

				arg := &EnvPluginArgs{}
				err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
				if err != nil {
					scope.Log("%s: %s", "environ", err.Error())
					return result
				}

				row := ordereddict.NewDict().
					SetDefault(&vfilter.Null{}).
					SetCaseInsensitive()
				if len(arg.Vars) == 0 {
					for _, env_var := range os.Environ() {
						parts := strings.SplitN(env_var, "=", 2)

						// Just hide ShadowedEnv but do not warn about
						// it since there is nothing the caller can do
						// to avoid it.
						if utils.InString(ShadowedEnv, parts[0]) {
							continue
						}

						if len(parts) > 1 {
							row.Set(parts[0], parts[1])
						}
					}
				} else {
					for _, env_var := range arg.Vars {
						value, pres := os.LookupEnv(env_var)
						if pres {
							if utils.InString(ShadowedEnv, env_var) {
								scope.Log("environ: access to env var %s is denied", env_var)
								continue
							}
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
