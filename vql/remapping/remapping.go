package remapping

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/yaml/v2"
	"www.velocidex.com/golang/velociraptor/accessors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
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
	elided := json.MustMarshalString(remapping_config)
	if len(elided) > 100 {
		elided = elided[:100] + " ..."
	}
	scope.Log("Applying remapping %v", elided)

	manager := accessors.GetManager(scope)
	if arg.Clear {
		manager.Clear()
	}

	global_device_manager := accessors.GetDefaultDeviceManager(
		config_obj).Copy()
	for _, cp := range arg.Copy {
		accessor, err := global_device_manager.GetAccessor(cp, scope)
		if err != nil {
			scope.Log("remap: %v", err)
			return vfilter.Null{}
		}

		manager.Register(cp, accessor, "")
	}

	// Reset the scope to default for remapping accessors.
	pristine_scope := scope.Copy()
	device_manager := accessors.GetDefaultDeviceManager(config_obj).Copy()
	pristine_scope.AppendVars(ordereddict.NewDict().
		Set(constants.SCOPE_DEVICE_MANAGER, device_manager))

	err = ApplyRemappingOnScope(ctx, config_obj, pristine_scope, scope, manager,
		ordereddict.NewDict(), remapping_config)
	if err != nil {
		// If we failed to install the remapping then we need to
		// ensure there is a null remapping installed. Otherwise VQL
		// code will continue to use the scope and may access the
		// original context instead of the remapped context. This may
		// lead to confusion as files will be read from the original
		// host not the remapped files.

		scope.Log("remap: %v", err)
		scope.Log("remap: Failed to apply remapping - will apply an empty remapping to block further processing")
		manager.Clear()
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
