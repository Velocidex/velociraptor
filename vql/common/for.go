package common

import (
	"context"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type ForPluginArgs struct {
	Var     string              `vfilter:"required,field=var,doc=The variable to assign."`
	Foreach vfilter.LazyExpr    `vfilter:"required,field=foreach,doc=The variable to iterate over."`
	Query   vfilter.StoredQuery `vfilter:"optional,field=query,doc=Run this query over the item."`
}

type ForPlugin struct{}

func (self ForPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &ForPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("for: %v", err)
			return
		}

		scope.Log("The for() plugin is deprecated. Please use foreach() instead.")

		// Force the in parameter to be a query.
		stored_query := arg.Foreach.ToStoredQuery(scope)
		for item := range stored_query.Eval(ctx, scope) {
			// Evaluate the query on the new value
			new_scope := scope.Copy()
			new_scope.AppendVars(ordereddict.NewDict().Set(
				arg.Var, item))

			for item := range arg.Query.Eval(ctx, new_scope) {
				select {
				case <-ctx.Done():
					return

				case output_chan <- item:
				}
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
