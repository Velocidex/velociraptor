package common

import (
	"context"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type _SwitchPlugin struct{}

func (self _SwitchPlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "switch", args)()

		queries := []vfilter.StoredQuery{}
		members := scope.GetMembers(args)

		for _, member := range members {
			v, pres := args.Get(member)
			if pres {
				queries = append(queries, arg_parser.ToStoredQuery(ctx, v))
			}
		}

		// Evaluate each query - the first query that returns
		// results will be emitted. We do not evaluate the
		// other queries at all.
		for _, query := range queries {
			found := false
			new_scope := scope.Copy()
			for item := range query.Eval(ctx, new_scope) {
				found = true
				select {
				case <-ctx.Done():
					return

				case output_chan <- item:
				}
			}

			if found {
				return
			}
		}
	}()

	return output_chan

}

func (self _SwitchPlugin) Info(
	scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:         "switch",
		Doc:          "Executes each query. The first query to return any rows will be emitted.",
		FreeFormArgs: true,
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_SwitchPlugin{})
}
