package aggregates

import (
	"context"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/functions"
	"www.velocidex.com/golang/vfilter/types"
)

type rateState struct {
	x float64
	y float64
}

type _RateFunctionArgs struct {
	X float64 `vfilter:"required,field=x,doc=The X float"`
	Y float64 `vfilter:"required,field=y,doc=The Y float"`
}

type _RateFunction struct {
	functions.Aggregator
}

func (self _RateFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "rate",
		Doc:     "Calculates the rate (derivative) between two quantities.",
		ArgType: type_map.AddType(scope, _RateFunctionArgs{}),
	}
}

// Aggregate functions must be copiable.
func (self _RateFunction) Copy() types.FunctionInterface {
	return _RateFunction{
		Aggregator: functions.NewAggregator(),
	}
}

func (self _RateFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &_RateFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("rate: %s", err.Error())
		return vfilter.Null{}
	}

	previous_value_any, pres := self.GetContext(scope)
	if !pres {
		self.SetContext(scope, &rateState{x: arg.X, y: arg.Y})
		return vfilter.Null{}
	}

	state := previous_value_any.(*rateState)
	value := (arg.X - state.x) / (arg.Y - state.y)
	self.SetContext(scope, &rateState{x: arg.X, y: arg.Y})

	return value
}

func init() {
	vql_subsystem.RegisterFunction(&_RateFunction{})
}
