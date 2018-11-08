package common

import (
	"context"

	"www.velocidex.com/golang/vfilter"
)

type WatchPluginArgs struct {
	Period int64               `vfilter:"required,field=period"`
	Query  vfilter.StoredQuery `vfilter:"required,field=query"`
}

type WatchPlugin struct{}

func (self WatchPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &WatchPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("watch: %v", err)
			return
		}
	}()

	return output_chan
}

func (self WatchPlugin) Info(type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "watch",
		Doc:     "Run query periodically and watch for changes in output.",
		ArgType: type_map.AddType(&WatchPluginArgs{}),
	}
}

func init() {
	// Not implemented yet.
	// vql_subsystem.RegisterPlugin(&WatchPlugin{})
}
