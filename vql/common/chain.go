package common

import (
	"context"
	"sync"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type _ChainPluginArgs struct {
	Async bool `vfilter:"optional,field=async,doc=If specified we run all queries asynchronously and combine the output."`
}

type _ChainPlugin struct{}

func (self _ChainPlugin) Info(scope types.Scope, type_map *types.TypeMap) *types.PluginInfo {
	return &types.PluginInfo{
		Name: "chain",
		Doc: "Chain the output of several queries into the same result set." +
			"This plugin takes any args and chains them.",
		ArgType:      type_map.AddType(scope, &_ChainPluginArgs{}),
		FreeFormArgs: true,
	}
}

func (self _ChainPlugin) Call(
	ctx context.Context,
	scope types.Scope,
	args *ordereddict.Dict) <-chan types.Row {
	output_chan := make(chan types.Row)

	queries := []types.StoredQuery{}

	// Retain the order of clauses according to their definition order
	members := scope.GetMembers(args)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "chain", args)()

		var async bool

		async_any, pres := args.Get("async")
		if pres && scope.Bool(async_any) {
			async = true
		}

		for _, member := range members {
			if member == "async" {
				continue
			}

			member_obj, pres := args.Get(member)
			if pres {
				queries = append(queries, arg_parser.ToStoredQuery(ctx, member_obj))
			}
		}

		wg := &sync.WaitGroup{}

		eval_query := func(query types.StoredQuery) {
			defer wg.Done()

			new_scope := scope.Copy()

			in_chan := query.Eval(ctx, new_scope)
			for item := range in_chan {
				select {
				case <-ctx.Done():
					new_scope.Close()
					return

				case output_chan <- item:
				}
			}

			new_scope.Close()
		}

		for _, query := range queries {
			wg.Add(1)
			if async {
				go eval_query(query)
			} else {
				eval_query(query)
			}
		}

		wg.Wait()
	}()

	return output_chan

}

type _CombinePlugin struct{}

func (self _CombinePlugin) Info(scope types.Scope, type_map *types.TypeMap) *types.PluginInfo {
	return &types.PluginInfo{
		Name: "combine",
		Doc: "Combine the output of several queries into the same result set." +
			"A convenience plugin acting like chain(async=TRUE).",
		ArgType: type_map.AddType(scope, _CombinePlugin{}),
	}
}

func (self _CombinePlugin) Call(
	ctx context.Context,
	scope types.Scope,
	args *ordereddict.Dict) <-chan types.Row {

	args.Set("async", true)
	return _ChainPlugin{}.Call(ctx, scope, args)
}

func init() {
	vql_subsystem.RegisterPlugin(&_CombinePlugin{})
	vql_subsystem.RegisterPlugin(&_ChainPlugin{})
}
