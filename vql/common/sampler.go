package common

import (
	"context"

	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type _SamplerPluginArgs struct {
	Query vfilter.StoredQuery `vfilter:"required,field=query,doc=Source query."`
	N     int64               `vfilter:"required,field=n,doc=Pick every n row from query."`
}

type _SamplerPlugin struct{}

func (self _SamplerPlugin) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &_SamplerPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("sample: %v", err)
			return
		}

		count := 0
		for row := range arg.Query.Eval(ctx, scope) {
			if count%int(arg.N) == 0 {
				output_chan <- row
			}
			count += 1
		}
	}()
	return output_chan
}

func (self _SamplerPlugin) Info(
	scope *vfilter.Scope,
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
