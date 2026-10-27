package bits

import (
	"context"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type _AndFunctionArgs struct {
	A uint64 `vfilter:"optional,field=a"`
	B uint64 `vfilter:"optional,field=b"`
}

type _AndFunction struct{}

func (self _AndFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "bit_and",
		Doc:     "Returns a bitwise AND operation between two integers.",
		ArgType: type_map.AddType(scope, &_AndFunctionArgs{}),
	}
}

func (self _AndFunction) Call(ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "bit_and", args)()

	arg := &_AndFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("bit_and: %s", err.Error())
		return vfilter.Null{}
	}

	return arg.A & arg.B
}

type _OrFunctionArgs struct {
	A uint64 `vfilter:"optional,field=a"`
	B uint64 `vfilter:"optional,field=b"`
}

type _OrFunction struct{}

func (self _OrFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "bit_or",
		Doc:     "Returns a bitwise OR operation between two integers.",
		ArgType: type_map.AddType(scope, &_OrFunctionArgs{}),
	}
}

func (self _OrFunction) Call(ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "bit_or", args)()

	arg := &_OrFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("bit_or: %s", err.Error())
		return vfilter.Null{}
	}

	return arg.A | arg.B
}

type _NotFunctionArgs struct {
	Value uint64 `vfilter:"optional,field=value"`
}

type _NotFunction struct{}

func (self _NotFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "bit_not",
		Doc:     "Returns a bitwise NOT operation on an integer.",
		ArgType: type_map.AddType(scope, &_NotFunctionArgs{}),
	}
}

func (self _NotFunction) Call(ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "bit_not", args)()

	arg := &_NotFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("bit_not: %s", err.Error())
		return vfilter.Null{}
	}

	return ^arg.Value
}

type _XORFunctionArgs struct {
	A uint64 `vfilter:"optional,field=a"`
	B uint64 `vfilter:"optional,field=b"`
}

type _XORFunction struct{}

func (self _XORFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "bit_xor",
		Doc:     "Returns a bitwise XOR operation on an integer.",
		ArgType: type_map.AddType(scope, &_XORFunctionArgs{}),
	}
}

func (self _XORFunction) Call(ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "bit_xor", args)()

	arg := &_XORFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("bit_xor: %s", err.Error())
		return vfilter.Null{}
	}

	return arg.A ^ arg.B
}

func init() {
	vql_subsystem.RegisterFunction(&_AndFunction{})
	vql_subsystem.RegisterFunction(&_OrFunction{})
	vql_subsystem.OverrideFunction(&_NotFunction{})
	vql_subsystem.OverrideFunction(&_XORFunction{})
}
