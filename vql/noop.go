package vql

import (
	"context"

	"github.com/Velocidex/ordereddict"
	vfilter "www.velocidex.com/golang/vfilter"
)

type NoopPlugin struct {
	OriginalPlugin vfilter.PluginGeneratorInterface
}

func (self NoopPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {

	scope.Log("plugin %s was replaced with noop, result will be empty", self.OriginalPlugin.Info(nil, nil).Name)

	channel := make(chan vfilter.Row)
	defer close(channel)

	return channel
}

func (self NoopPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return self.OriginalPlugin.Info(scope, type_map)
}
