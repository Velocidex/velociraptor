package aggregates

import (
	"context"
	"reflect"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type _AnyFunction struct{}

func (self _AnyFunction) Info(scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "any",
		Doc:     "Returns TRUE if any items are true.",
		ArgType: type_map.AddType(scope, _AllFunctionArgs{}),
	}
}

func evalAnyCondition(
	ctx context.Context,
	scope vfilter.Scope,
	arg *_AllFunctionArgs,
	value vfilter.Any) bool {

	// If a list of regex is given then we match if any of the regex
	// match - this is a convenience for the regex alternate operator
	// (X|Y|Z).
	if len(arg.Regex) > 0 {
		for _, regex := range arg.Regex {
			if scope.Match(regex, value) {
				return true
			}
		}
		return false
	}

	if arg.Filter != nil {
		return scope.Bool(arg.Filter.Reduce(ctx, scope, []types.Any{value}))
	}

	return false
}

func (self _AnyFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &_AllFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("any: %v", err)
		return vfilter.Null{}
	}

	if len(arg.Regex) == 0 && arg.Filter == nil {
		scope.Log("any: One of filter or regex must be provided")
		return vfilter.Null{}
	}

	// Walk over any items and evaluate them
	switch t := arg.Items.(type) {
	case types.LazyExpr:
		arg.Items = t.Reduce(ctx)

	case types.StoredQuery:
		for row := range t.Eval(ctx, scope) {
			// Evaluate the row with the callback
			triggered := evalAnyCondition(ctx, scope, arg, row)
			if triggered {
				return true
			}
		}
		return false
	}

	a_value := reflect.Indirect(reflect.ValueOf(arg.Items))
	a_type := a_value.Type()

	if a_type.Kind() == reflect.Slice {
		for i := 0; i < a_value.Len(); i++ {
			element := a_value.Index(i).Interface()
			triggered := evalAnyCondition(ctx, scope, arg, element)
			if triggered {
				return true
			}
		}
		return false
	}

	// It is not a slice but maybe it is dict like: has the
	// Associative protocol.
	members := scope.GetMembers(arg.Items)
	if len(members) > 0 {
		for _, item := range members {
			value, pres := scope.Associative(arg.Items, item)
			if pres {
				triggered := evalAnyCondition(ctx, scope, arg, value)
				if triggered {
					return true
				}
			}
		}
		return false
	}

	// We dont know what the item actually is - let the callback tell us
	return evalAnyCondition(ctx, scope, arg, arg.Items)
}

func init() {
	vql_subsystem.RegisterFunction(&_AnyFunction{})
}
