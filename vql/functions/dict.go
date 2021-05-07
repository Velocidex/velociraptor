package functions

import (
	"context"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type _ToDictFunctionArgs struct {
	Item vfilter.Any `vfilter:"optional,field=item"`
}

// A helper function to build a dict within the query.
// e.g. dict(foo=5, bar=6)
type _ToDictFunc struct{}

func (self _ToDictFunc) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "to_dict",
		Doc:     "Construct a dict from another object. If items is a query use _key and _value columns to set the dict's keys and values.",
		ArgType: type_map.AddType(scope, &_ToDictFunctionArgs{}),
	}
}

func (self _ToDictFunc) Call(ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &_ToDictFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("to_dict: %s", err.Error())
		return vfilter.Null{}
	}

	switch t := arg.Item.(type) {

	// A stored query expects rows with _key and _value columns
	case vfilter.StoredQuery:
		result := ordereddict.NewDict()
		for row_item := range t.Eval(ctx, scope) {
			key := vql_subsystem.GetStringFromRow(scope, row_item, "_key")
			if key == "" {
				continue
			}

			value, pres := scope.Associative(row_item, "_value")
			if !pres {
				value = vfilter.Null{}
			}
			result.Set(key, value)
		}
		return result
	default:
		return vfilter.RowToDict(ctx, scope, arg.Item)
	}

}

type _ItemsFunc struct{}

func (self _ItemsFunc) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "items",
		Doc:     "Iterate over dict members producing _key and _value columns",
		ArgType: type_map.AddType(scope, &_ToDictFunctionArgs{}),
	}
}

func (self _ItemsFunc) Call(ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &_ToDictFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("items: %s", err.Error())
		return vfilter.Null{}
	}

	result := []*ordereddict.Dict{}

	switch t := arg.Item.(type) {
	case *ordereddict.Dict:
		for _, k := range t.Keys() {
			v, _ := t.Get(k)
			result = append(result, ordereddict.NewDict().
				Set("_key", k).Set("_value", v))
		}
	default:
		result = append(result, ordereddict.NewDict().
			Set("_value", arg.Item))
	}

	return result
}

func init() {
	vql_subsystem.RegisterFunction(&_ItemsFunc{})
	vql_subsystem.RegisterFunction(&_ToDictFunc{})
}
