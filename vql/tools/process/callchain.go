// Query the tracker and get the process call chain.

package process

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type getChainArgs struct {
	Id string `vfilter:"required,field=id,doc=Process ID."`
}

type getChain struct {
	tracker *ProcessTracker
}

func (self getChain) Copy() types.FunctionInterface {
	return self
}

func (self getChain) Call(ctx context.Context,
	scope types.Scope, args *ordereddict.Dict) types.Any {

	arg := &getChainArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("tracker.GetChain: %v", err)
		return vfilter.Null{}
	}

	return self.tracker.CallChain(arg.Id)
}

func (self getChain) Info(scope types.Scope,
	type_map *types.TypeMap) *types.FunctionInfo {
	return &types.FunctionInfo{}
}
