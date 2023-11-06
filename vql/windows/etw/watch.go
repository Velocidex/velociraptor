//go:build windows && cgo
// +build windows,cgo

package etw

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	"golang.org/x/sys/windows"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type WatchETWArgs struct {
	Name        string `vfilter:"optional,field=name,doc=A session name "`
	Provider    string `vfilter:"required,field=guid,doc=A Provider GUID to watch "`
	AnyKeywords uint64 `vfilter:"optional,field=any,doc=Any Keywords "`
	AllKeywords uint64 `vfilter:"optional,field=all,doc=All Keywords "`
	Level       int64  `vfilter:"optional,field=level,doc=Log level (0-5)"`
}

type WatchETWPlugin struct{}

func (self WatchETWPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &WatchETWArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("watch_etw: %s", err.Error())
			return
		}

		// Check that we have a valid GUID.
		wGuid, err := windows.GUIDFromString(arg.Provider)
		if err != nil {
			scope.Log("watch_etw: %s", err.Error())
			return
		}

		// By default listen to DEBUG level logs
		if arg.Level == 0 {
			arg.Level = 5
		}

		// Select a default session name
		if arg.Name == "" {
			arg.Name = "Velociraptor"
		}

		for {
			cancel, event_channel, err := GlobalEventTraceService.Register(
				ctx, scope, arg.Provider, arg.Name,
				arg.AnyKeywords, arg.AllKeywords, arg.Level,
				wGuid)
			if err != nil {
				scope.Log("watch_etw: %v", err)
				cancel()
				scope.Log("ETW session interrupted, will retry again.")
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Minute):
					continue
				}
			}
			defer cancel()

			// Wait until the query is complete.
			for event := range event_channel {
				select {
				case <-ctx.Done():
					return
				case output_chan <- event:
				}
			}
		}

	}()

	return output_chan
}

func (self WatchETWPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "watch_etw",
		Doc:     "Watch for events from an ETW provider.",
		ArgType: type_map.AddType(scope, &WatchETWArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&WatchETWPlugin{})
}
