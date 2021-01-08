// +build server_vql

package server

import (
	"context"
	"fmt"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
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

type _RateFunction struct{}

func (self _RateFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "rate",
		Doc:     "Calculates the rate (derivative) between two quantities.",
		ArgType: type_map.AddType(scope, _RateFunctionArgs{}),
	}
}

func (self _RateFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &_RateFunctionArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("count: %s", err.Error())
		return vfilter.Null{}
	}

	previous_value_any, pres := scope.GetContext(GetID(&self))
	if !pres {
		scope.SetContext(GetID(&self), &rateState{x: arg.X, y: arg.Y})
		return vfilter.Null{}
	}

	state := previous_value_any.(*rateState)
	value := (arg.X - state.x) / (arg.Y - state.y)
	scope.SetContext(GetID(&self), &rateState{x: arg.X, y: arg.Y})

	return value
}

func init() {
	vql_subsystem.RegisterFunction(&_RateFunction{})
}

// Returns a unique ID for the object.
func GetID(obj types.Any) string {
	return fmt.Sprintf("%p", obj)
}
