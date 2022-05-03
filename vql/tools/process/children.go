package process

import (
	"context"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type getChildren struct{}

func (self getChildren) Call(ctx context.Context,
	scope types.Scope, args *ordereddict.Dict) types.Any {

	arg := &getChainArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("process_tracker_children: %v", err)
		return vfilter.Null{}
	}

	tracker := GetGlobalTracker()
	if tracker == nil {
		scope.Log("process_tracker_children: Initialize a process tracker first with process_tracker_install()")
		return &vfilter.Null{}
	}

	return tracker.Children(arg.Id)
}

func (self getChildren) Info(scope types.Scope,
	type_map *types.TypeMap) *types.FunctionInfo {
	return &types.FunctionInfo{
		Name:    "process_tracker_children",
		Doc:     "Get all children of a process.",
		ArgType: type_map.AddType(scope, &getChainArgs{}),
	}
}

type getAll struct{}

func (self getAll) Call(ctx context.Context,
	scope types.Scope, args *ordereddict.Dict) types.Any {

	tracker := GetGlobalTracker()
	if tracker == nil {
		scope.Log("process_tracker_all: Initialize a process tracker first with process_tracker_install()")
		return &vfilter.Null{}
	}

	return tracker.Processes()
}

func (self getAll) Info(scope types.Scope,
	type_map *types.TypeMap) *types.FunctionInfo {
	return &types.FunctionInfo{
		Name: "process_tracker_all",
		Doc:  "Get all prcesses stored in the tracker.",
	}
}

func init() {
	vql_subsystem.RegisterFunction(&getChildren{})
	vql_subsystem.RegisterFunction(&getAll{})
}
