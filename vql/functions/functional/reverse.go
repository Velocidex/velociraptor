package functional

import (
	"context"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type _ReverseFunctionArgs struct {
	Items []vfilter.Any `vfilter:"required,field=items,doc=A list of items to reverse"`
}

type _ReverseFunction struct{}

func (self _ReverseFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "reverse",
		Doc:     "reverse an array.",
		ArgType: type_map.AddType(scope, &_ReverseFunctionArgs{}),
	}
}

func (self _ReverseFunction) Call(ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "reverse", args)()

	arg := &_ReverseFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("reverse: %s", err.Error())
		return vfilter.Null{}
	}

	if len(arg.Items) == 0 {
		return []vfilter.Any{}
	}

	result := make([]vfilter.Any, 0, len(arg.Items))
	for i := len(arg.Items) - 1; i >= 0; i-- {
		result = append(result, arg.Items[i])
	}

	return result
}

func init() {
	vql_subsystem.RegisterFunction(&_ReverseFunction{})

}
