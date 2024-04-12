package functions

import (
	"context"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type EvalFunctionArg struct {
	Func *vfilter.Lambda `vfilter:"required,field=func,doc=Lambda function to evaluate e.g. x=>1+1 where x will be the current scope."`
}

type EvalFunction struct{}

func (self EvalFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "eval",
		Doc:     "Evaluate a vql lambda function on the current scope.",
		ArgType: type_map.AddType(scope, EvalFunctionArg{}),
	}
}

func (self EvalFunction) Call(ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &EvalFunctionArg{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("eval: %s", err.Error())
		return vfilter.Null{}
	}

	// Evaluate the lambda on the current scope.
	res := arg.Func.Reduce(ctx, scope, []vfilter.Any{scope})

	// Materialize the lambda
	return vql_subsystem.Materialize(ctx, scope, res)
}

func init() {
	vql_subsystem.RegisterFunction(&EvalFunction{})
}
