package functions

import (
	"context"
	"reflect"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type EvalFunctionArg struct {
	Func *vfilter.Lambda `vfilter:"required,field=func,doc=Lambda function to evaluate e.g. x=>1+1 where x will be the current scope."`
	Args vfilter.Any     `vfilter:"optional,field=args,doc=An array of elements to use as args for the lambda function. If not provided we pass the scope"`
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

	defer vql_subsystem.RegisterMonitor(ctx, "eval", args)()

	arg := &EvalFunctionArg{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("eval: %s", err.Error())
		return vfilter.Null{}
	}

	var lambda_args []vfilter.Any
	if arg.Args != nil {
		slice := reflect.ValueOf(arg.Args)

		// A slice of strings.
		if slice.Type().Kind() != reflect.Slice {
			lambda_args = append(lambda_args,
				vql_subsystem.Materialize(ctx, scope, arg.Args))
		} else {
			for i := 0; i < slice.Len(); i++ {
				value := slice.Index(i).Interface()
				lambda_args = append(lambda_args,
					vql_subsystem.Materialize(ctx, scope, value))
			}
		}

	} else {
		lambda_args = append(lambda_args, scope)
	}

	// Evaluate the lambda on the current scope.
	res := arg.Func.Reduce(ctx, scope, lambda_args)

	// Materialize the lambda
	return vql_subsystem.Materialize(ctx, scope, res)
}

func init() {
	vql_subsystem.RegisterFunction(&EvalFunction{})
}
