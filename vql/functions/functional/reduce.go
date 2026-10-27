package functional

import (
	"context"
	"fmt"
	"reflect"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/functions"
	"www.velocidex.com/golang/vfilter/types"
)

type _ReduceFunctionArgs struct {
	Items              vfilter.Any     `vfilter:"required,field=items,doc=A list of items to apply the lambda on"`
	Lambda             *vfilter.Lambda `vfilter:"optional,field=lambda,doc=A VQL Lambda to apply on each item"`
	InitialAccumulator vfilter.Any     `vfilter:"optional,field=accum,doc=An initial value of the accumulator"`
}

type _ReduceFunction struct {
	functions.Aggregator
	id string
}

func (self _ReduceFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "reduce",
		Doc:     "Apply a lambda function on each item and return the accumulator. The lambda is expected to receive the accumulator and the item. For example `(acc, x)=>acc + x`",
		ArgType: type_map.AddType(scope, &_MapFunctionArgs{}),
	}
}

// Aggregate functions must be copiable.
func (self _ReduceFunction) Copy() types.FunctionInterface {
	return _ReduceFunction{
		Aggregator: functions.NewAggregator(),
		id:         fmt.Sprintf("id_%v", utils.GetId()),
	}
}

func (self _ReduceFunction) Call(ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "reduce", args)()

	arg := &_ReduceFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("reduce: %s", err.Error())
		return vfilter.Null{}
	}

	if arg.Items == nil || arg.Lambda == nil {
		return vfilter.Null{}
	}

	parameters := arg.Lambda.GetParameters()
	if len(parameters) != 2 && len(parameters) != 3 {
		scope.Log("reduce: lambda parameters are %v but must accept 2 or 3 "+
			"parameters: `acc, value, i=> ...` or `acc, value=> ...`",
			parameters)
	}
	value := vql_subsystem.Materialize(ctx, scope, arg.Items)

	calculate := func(accumulator, value vfilter.Any, idx int) vfilter.Any {
		// If the accumulator arg is not provided, we assign
		// the first item to the accumulator and **not** run
		// the lambda on it.
		if accumulator == nil {
			return value
		}

		call_params := []vfilter.Any{
			accumulator,
			value,
		}
		if len(parameters) == 3 {
			call_params = append(call_params, idx)
		}
		return arg.Lambda.Reduce(ctx, scope, call_params)
	}

	return scope.GetAggregatorCtx().Modify(self.id,
		func(accumulator types.Any, pres bool) types.Any {
			if !pres || accumulator == nil {
				accumulator = arg.InitialAccumulator
			}

			a_value := reflect.Indirect(reflect.ValueOf(value))
			a_type := a_value.Type()
			if a_type.Kind() == reflect.Slice {
				for i := 0; i < a_value.Len(); i++ {
					accumulator = calculate(
						accumulator, a_value.Index(i).Interface(), i)
				}
			} else {
				accumulator = calculate(accumulator, value, 0)
			}

			return accumulator
		})
}

func init() {
	vql_subsystem.RegisterFunction(&_ReduceFunction{})
}
