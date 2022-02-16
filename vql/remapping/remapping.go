package remapping

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/yaml/v2"
	"www.velocidex.com/golang/velociraptor/accessors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type RemappingArgs struct {
	Configuration string   `vfilter:"required,field=config,doc=A Valid remapping configuration in YAML format"`
	Copy          []string `vfilter:"optional,field=copy,doc=Accessors to copy to the new scope"`
	Clear         bool     `vfilter:"optional,field=clear,doc=If set we clear all accessors from the device manager"`
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

	global_device_manager := accessors.GlobalDeviceManager.Copy()
	for _, cp := range arg.Copy {
		accessor, err := global_device_manager.GetAccessor(cp, scope)
		if err != nil {
			scope.Log("remap: %v", err)
			return vfilter.Null{}
		}

		manager.Register(cp, accessor, "")
	}

	// Reset the scope to default for remapping accessors.
	subscope := scope.Copy()
	subscope.AppendVars(ordereddict.NewDict().
		Set(constants.SCOPE_DEVICE_MANAGER,
			accessors.GlobalDeviceManager.Copy()))

	err = ApplyRemappingOnScope(ctx, subscope, manager,
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
