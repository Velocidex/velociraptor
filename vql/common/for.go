package common

import (
	"context"

	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type ForPluginArgs struct {
	Var     string              `vfilter:"required,field=var,doc=The variable to assign."`
	Foreach vfilter.Any         `vfilter:"required,field=foreach,doc=The variable to iterate over."`
	Query   vfilter.StoredQuery `vfilter:"optional,field=query,doc=Run this query over the item."`
}

type ForPlugin struct{}

func (self ForPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &ForPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("for: %v", err)
			return
		}

		// Expand lazy expressions.
		lazy_v, ok := arg.Foreach.(vfilter.LazyExpr)
		if ok {
			arg.Foreach = lazy_v.Reduce()
		}

		// Force the in parameter to be a query.
		stored_query, ok := arg.Foreach.(vfilter.StoredQuery)
		if !ok {
			wrapper := vfilter.StoredQueryWrapper{arg.Foreach}
			stored_query = &wrapper
		}

		for item := range stored_query.Eval(ctx, scope) {
			// Evaluate the query on the new value
			new_scope := scope.Copy()
			new_scope.AppendVars(vfilter.NewDict().Set(
				arg.Var, item))

			for item := range arg.Query.Eval(ctx, new_scope) {
				output_chan <- item
			}
		}

	}()

	return output_chan
}

func (self ForPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "for",
		Doc:     "Iterate over a list.",
		ArgType: type_map.AddType(scope, &ForPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&ForPlugin{})
}
