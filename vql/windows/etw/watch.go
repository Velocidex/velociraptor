// +build windows,cgo

package etw

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"github.com/bi-zone/etw"
	"golang.org/x/sys/windows"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type WatchETWArgs struct {
	Provider string `vfilter:"required,field=guid,doc=A Provider GUID to watch "`
}

type WatchETWPlugin struct{}

func (self WatchETWPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &WatchETWArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("watch_etw: %s", err.Error())
			return
		}

		guid, err := windows.GUIDFromString(arg.Provider)
		if err != nil {
			scope.Log("watch_etw: %s", err.Error())
			return
		}
		session, err := etw.NewSession(guid)
		if err != nil {
			scope.Log("watch_etw: %s", err.Error())
			return
		}
		defer session.Close()

		cb := func(e *etw.Event) {
			event := ordereddict.NewDict().
				Set("System", e.Header)
			data, err := e.EventProperties()
			if err == nil {
				event.Set("EventData", data)
			}

			select {
			case <-ctx.Done():
			case output_chan <- event:
			}
		}

		sub_ctx, cancel := context.WithCancel(ctx)

		go func() {
			// When session.Process() exits, we exit the
			// query.
			defer cancel()
			err := session.Process(cb)
			if err != nil {
				scope.Log("watch_etw: %v", err)
			}
		}()

		// Wait here until the query is cancelled.
		<-sub_ctx.Done()
	}()

	return output_chan

}

func (self WatchETWPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "watch_etw",
		Doc:     "Watch for events from an ETW provider.",
		ArgType: type_map.AddType(scope, &WatchETWArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&WatchETWPlugin{})
}
