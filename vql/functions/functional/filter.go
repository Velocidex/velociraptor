package functional

import (
	"context"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type FilterFunctionArgs struct {
	List      []vfilter.Any   `vfilter:"required,field=list,doc=A list of items to filter"`
	Regex     string          `vfilter:"optional,field=regex,doc=A regex to test each item"`
	Condition *vfilter.Lambda `vfilter:"optional,field=condition,doc=A VQL lambda to use to filter elements"`
}

type FilterFunction struct{}

func (self *FilterFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	defer vql_subsystem.RegisterMonitor(ctx, "filter", args)()
	arg := &FilterFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("filter: %s", err.Error())
		return &vfilter.Null{}
	}

	if arg.Condition != nil {
		if arg.Regex != "" {
			scope.Log("ERROR:filter: Both regex and condition are specified - Will only use the condition and ignore regex!")
		}

		result := []types.Any{}
		for _, item := range arg.List {
			if scope.Bool(arg.Condition.Reduce(ctx, scope, []vfilter.Any{item})) {
				result = append(result, item)
			}
		}
		return result
	}

	result := []types.Any{}
	for _, item := range arg.List {
		if scope.Match(arg.Regex, item) {
			result = append(result, item)
		}
	}
	return result
}

func (self FilterFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "filter",
		Doc:     "Filters a strings array by regex.",
		ArgType: type_map.AddType(scope, &FilterFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&FilterFunction{})
}
