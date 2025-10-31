package debug

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/utils/faults"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type FaultSetFunctionArgs struct {
	HTTPBlock int64 `vfilter:"optional,field=http_block,doc=Set delay in seconds on HTTP connections (-1 to remove)"`
	TimeStep  int64 `vfilter:"optional,field=time_step,doc=Set time step on now()  (-1 to remove)"`
}

type FaultSetFunction struct{}

func (self *FaultSetFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &FaultSetFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("fault: %s", err.Error())
		return false
	}

	if arg.HTTPBlock > 0 {
		faults.FaultInjector.SetBlockHTTPDo(
			time.Duration(arg.HTTPBlock) * time.Second)
	}

	if arg.HTTPBlock < 0 {
		faults.FaultInjector.SetBlockHTTPDo(0)
	}

	if arg.TimeStep == -1 {
		faults.FaultInjector.SetTimeStep(0)

	} else if arg.TimeStep != 0 {
		faults.FaultInjector.SetTimeStep(
			time.Duration(arg.TimeStep) * time.Second)
	}

	return false
}

func (self *FaultSetFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "fault",
		Doc:      "Set faults in the fault injector.",
		ArgType:  type_map.AddType(scope, &FaultSetFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions().Build(),
	}
}

// Plugin is only available in debug mode.
func AddDebugPlugins(config_obj *config_proto.Config) {
	if !config_obj.DebugMode {
		return
	}

	vql_subsystem.RegisterFunction(&FaultSetFunction{})
}
