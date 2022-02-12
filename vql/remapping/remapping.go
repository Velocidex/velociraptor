package remapping

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/yaml/v2"
	"www.velocidex.com/golang/velociraptor/accessors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type RemappingArgs struct {
	Configuration string `vfilter:"required,field=config,doc=A Valid remapping configuration in YAML format"`
	Clear         bool   `vfilter:"optional,field=clear,doc=If set we clear all accessors from the device manager"`
}

type RemappingFunc struct{}

func (self RemappingFunc) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &RemappingArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("remap: %s", err.Error())
		return false
	}

	config_obj := &config_proto.Config{}
	err = yaml.UnmarshalStrict([]byte(arg.Configuration), config_obj)
	if err != nil {
		scope.Log("remap: %v", err)
		return vfilter.Null{}
	}

	remapping_config := config_obj.Remappings
	scope.Log("Applying remapping %v", remapping_config)

	manager := accessors.GetManager(scope)
	if arg.Clear {
		manager.Clear()
	}
	err = ApplyRemappingOnScope(ctx, scope, manager,
		ordereddict.NewDict(), remapping_config)
	if err != nil {
		scope.Log("remap: %v", err)
		return vfilter.Null{}
	}

	return remapping_config
}

func (self RemappingFunc) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "remap",
		Doc:     "Apply a remapping configuration to the root scope.",
		ArgType: type_map.AddType(scope, &RemappingArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&RemappingFunc{})
}
