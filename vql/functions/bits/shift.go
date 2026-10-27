package bits

import (
	"context"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type _ShrFunctionArgs struct {
	Value uint64 `vfilter:"optional,field=value"`
	Bits  uint64 `vfilter:"optional,field=bits"`
}

type _ShrFunction struct{}

func (self _ShrFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "bit_shr",
		Doc:     "Shifts the value to the right.",
		ArgType: type_map.AddType(scope, &_ShrFunctionArgs{}),
	}
}

func (self _ShrFunction) Call(ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "bit_shr", args)()

	arg := &_ShrFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("bit_shr: %s", err.Error())
		return vfilter.Null{}
	}

	return arg.Value >> arg.Bits
}

type _ShlFunctionArgs struct {
	Value uint64 `vfilter:"optional,field=value"`
	Bits  uint64 `vfilter:"optional,field=bits"`
}

type _ShlFunction struct{}

func (self _ShlFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "bit_shl",
		Doc:     "Shifts the value to the left.",
		ArgType: type_map.AddType(scope, &_ShlFunctionArgs{}),
	}
}

func (self _ShlFunction) Call(ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "bit_shl", args)()

	arg := &_ShlFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("bit_shl: %s", err.Error())
		return vfilter.Null{}
	}

	return arg.Value << arg.Bits
}

func init() {
	vql_subsystem.RegisterFunction(&_ShlFunction{})
	vql_subsystem.RegisterFunction(&_ShrFunction{})
}
