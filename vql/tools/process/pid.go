package process

import (
	"context"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type getProcess struct{}

func (self getProcess) Call(ctx context.Context,
	scope types.Scope, args *ordereddict.Dict) types.Any {
	defer vql_subsystem.RegisterMonitor(ctx, "process_tracker_get", args)()

	arg := &getChainArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("process_tracker_get: %v", err)
		return &vfilter.Null{}
	}

	tracker := GetGlobalTracker()
	if tracker == nil {
		scope.Log("process_tracker_get: Initialize a process tracker first with process_tracker_install()")
		return &vfilter.Null{}
	}

	entry, pres := tracker.Enrich(ctx, scope, arg.Id)
	if pres {
		return entry
	}

	return &vfilter.Null{}
}

func (self getProcess) Info(scope types.Scope,
	type_map *types.TypeMap) *types.FunctionInfo {
	return &types.FunctionInfo{
		Name:    "process_tracker_get",
		Doc:     "Get a single process from the global tracker.",
		ArgType: type_map.AddType(scope, &getChainArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&getProcess{})
}
