package common

import (
	"context"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type _SamplerPluginArgs struct {
	Query vfilter.StoredQuery `vfilter:"required,field=query,doc=Source query."`
	N     int64               `vfilter:"required,field=n,doc=Pick every n row from query."`
}

type _SamplerPlugin struct{}

func (self _SamplerPlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "sample", args)()

		arg := &_SamplerPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("sample: %v", err)
			return
		}

		if arg.N == 0 {
			arg.N = 1
		}

		count := 0
		for row := range arg.Query.Eval(ctx, scope) {
			if count%int(arg.N) == 0 {
				select {
				case <-ctx.Done():
					return

				case output_chan <- row:
				}
			}
			count += 1
		}
	}()
	return output_chan
}

func (self _SamplerPlugin) Info(
	scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "sample",
		Doc:  "Executes 'query' and samples every n'th row.",

		ArgType: type_map.AddType(scope, &_SamplerPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_SamplerPlugin{})
}
