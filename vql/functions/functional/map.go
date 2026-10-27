package functional

import (
	"context"
	"reflect"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type _MapFunctionArgs struct {
	Items  vfilter.Any     `vfilter:"required,field=items,doc=A list of items to apply the lambda on"`
	Lambda *vfilter.Lambda `vfilter:"optional,field=lambda,doc=A VQL Lambda to apply on each item"`
}

type _MapFunction struct{}

func (self _MapFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "map",
		Doc:     "Apply a map on a list of items.",
		ArgType: type_map.AddType(scope, &_MapFunctionArgs{}),
	}
}

func (self _MapFunction) Call(ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "map", args)()

	arg := &_MapFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("map: %s", err.Error())
		return vfilter.Null{}
	}

	parameters := arg.Lambda.GetParameters()
	if len(parameters) != 1 && len(parameters) != 2 {
		scope.Log("map: lambda parameters are %v but must accept 1 or 2 "+
			"parameters: `value=> ...` or `value, idx=> ...`",
			parameters)
	}

	result := []vfilter.Any{}
	calculate := func(item vfilter.Any, idx int) vfilter.Any {
		call_params := []vfilter.Any{item}
		if len(parameters) == 2 {
			call_params = append(call_params, idx)
		}
		return arg.Lambda.Reduce(ctx, scope, call_params)
	}

	if arg.Items != nil {
		value := vql_subsystem.Materialize(ctx, scope, arg.Items)

		a_value := reflect.Indirect(reflect.ValueOf(value))
		a_type := a_value.Type()
		if a_type.Kind() == reflect.Slice {
			for i := 0; i < a_value.Len(); i++ {
				item := a_value.Index(i).Interface()
				result = append(result, calculate(item, i))
			}
		} else {
			result = append(result, calculate(arg.Items, 0))
		}
	}

	return result
}

func init() {
	vql_subsystem.RegisterFunction(&_MapFunction{})

}
