package functions

import (
	"context"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type _ToDictFunctionArgs struct {
	Item vfilter.Any `vfilter:"optional,field=item"`
}

// A helper function to build a dict within the query.
// e.g. dict(foo=5, bar=6)
type _ToDictFunc struct{}

func (self _ToDictFunc) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name: "to_dict",
		Doc:  "Construct a dict from another object.",
	}
}

func (self _ToDictFunc) Call(ctx context.Context, scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &_ToDictFunctionArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("to_dict: %s", err.Error())
		return vfilter.Null{}
	}

	return vfilter.RowToDict(ctx, scope, arg.Item)
}

func init() {
	vql_subsystem.RegisterFunction(&_ToDictFunc{})
}
