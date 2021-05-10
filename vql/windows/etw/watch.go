// +build windows,cgo

package etw

import (
	"context"
	"sync"

	"github.com/Velocidex/ordereddict"
	"github.com/bi-zone/etw"
	"golang.org/x/sys/windows"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type WatchETWArgs struct {
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

		// By default listen to DEBUG level logs
		if arg.Level == 0 {
			arg.Level = 5
		}

		guid, err := windows.GUIDFromString(arg.Provider)
		if err != nil {
			scope.Log("watch_etw: %s", err.Error())
			return
		}
		session, err := etw.NewSession(guid, func(cfg *etw.SessionOptions) {
			cfg.MatchAnyKeyword = arg.AnyKeywords
			cfg.MatchAllKeyword = arg.AllKeywords
			cfg.Level = etw.TraceLevel(arg.Level)
		})

		if err != nil {
			scope.Log("watch_etw: %s", err.Error())
			return
		}

		sub_ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		cancelled := false

		cb := func(e *etw.Event) {
			if cancelled {
				return
			}

			event := ordereddict.NewDict().
				Set("System", e.Header)
			data, err := e.EventProperties()
			if err == nil {
				event.Set("EventData", data)
			}

			select {
			case <-sub_ctx.Done():
				return
			case output_chan <- event:
			}
		}

		wg := &sync.WaitGroup{}
		wg.Add(1)

		go func() {
			defer wg.Done()

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

		cancelled = true
		err = session.Close()
		if err != nil {
			scope.Log("watch_etw: %v", err)
			return
		}

		// Wait here for session.Process() to finish - there
		// may be a queue of events to send to the callback
		// that ETW will try to clear so we need to wait here
		// until it does.
		wg.Wait()

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
