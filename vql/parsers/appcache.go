package parsers

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/regparser/appcompatcache"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type AppCompatCacheArgs struct {
	Value string `vfilter:"required,field=value,doc=Binary data to parse."`
}

type AppCompatCache struct{}

func (self AppCompatCache) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		arg := AppCompatCacheArgs{}
		err := vfilter.ExtractArgs(scope, args, &arg)
		if err != nil {
			scope.Log("AppCompatCache: %v", err)
			return
		}

		for _, item := range appcompatcache.ParseValueData([]byte(arg.Value)) {
			output_chan <- item
		}

	}()
	return output_chan
}

func (self AppCompatCache) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "appcompatcache",
		Doc:     "Parses the appcompatcache.",
		ArgType: type_map.AddType(scope, &AppCompatCache{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&AppCompatCache{})

}
