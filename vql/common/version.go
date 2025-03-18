package common

import (
	"context"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type GetVersionArgs struct {
	Function string `vfilter:"optional,field=function"`
	Plugin   string `vfilter:"optional,field=plugin"`
}

type GetVersion struct{}

func (self GetVersion) Info(scope types.Scope, type_map *types.TypeMap) *types.FunctionInfo {
	return &types.FunctionInfo{
		Name:    "version",
		Doc:     "Gets the version of a VQL plugin or function.",
		ArgType: type_map.AddType(scope, &GetVersionArgs{}),
	}
}

func (self GetVersion) Call(
	ctx context.Context,
	scope types.Scope, args *ordereddict.Dict) types.Any {
	arg := &GetVersionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("version: %v", err)
		return types.Null{}
	}

	if arg.Plugin != "" {
		plugin, pres := scope.GetPlugin(arg.Plugin)
		if !pres {
			return types.Null{}
		}

		type_map := types.NewTypeMap()
		info := plugin.Info(scope, type_map)

		if info.Version < 0 {
			return types.Null{}
		}

		return info.Version

	} else if arg.Function != "" {
		function, pres := scope.GetFunction(arg.Function)
		if !pres {
			return types.Null{}
		}
		type_map := types.NewTypeMap()
		info := function.Info(scope, type_map)
		if info.Version < 0 {
			return types.Null{}
		}

		return info.Version
	}
	scope.Log("version: One of plugin or function should be specified")

	return 0
}

func init() {
	vql_subsystem.OverrideFunction(&GetVersion{})
}
