package common

import (
	"context"
	"os"
	"strings"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type EnvPluginArgs struct {
	Vars []string `vfilter:"optional,field=vars"`
}

type EnvFunctionArgs struct {
	Var string `vfilter:"required,field=var"`
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

func (self *EnvFunction) Info(type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "environ",
		Doc:     "Get an environment variable.",
		ArgType: type_map.AddType(&EnvFunctionArgs{}),
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
