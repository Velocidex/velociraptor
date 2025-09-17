// Query the tracker and get the process call chain.

package process

import (
	"context"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type getChainArgs struct {
	Id       string `vfilter:"required,field=id,doc=Process ID."`
	MaxItems int64  `vfilter:"optional,field=max_items,doc=The maximum number of process entries to return (default 10)"`
}

type getChain struct{}

func (self getChain) Call(ctx context.Context,
	scope types.Scope, args *ordereddict.Dict) types.Any {
	defer vql_subsystem.RegisterMonitor(ctx, "process_tracker_callchain", args)()

	arg := &getChainArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("process_tracker_callchain: %v", err)
		return vfilter.Null{}
	}

	tracker := GetGlobalTracker()
	if tracker == nil {
		scope.Log("process_tracker_callchain: Initialize a process tracker first with process_tracker_install()")
		return &vfilter.Null{}
	}

	return tracker.CallChain(ctx, scope, arg.Id, arg.MaxItems)
}

func (self getChain) Info(scope types.Scope,
	type_map *types.TypeMap) *types.FunctionInfo {
	return &types.FunctionInfo{
		Name:    "process_tracker_callchain",
		Doc:     "Get a call chain from the global process tracker.",
		ArgType: type_map.AddType(scope, &getChainArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&getChain{})
}
