// +build windows,cgo

package etw

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/bi-zone/etw"
	"golang.org/x/sys/windows"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

var (
	id uint64
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

		// By default listen to DEBUG level logs
		if arg.Level == 0 {
			arg.Level = 5
		}

		// Select a default session name
		if arg.Name == "" {
			new_id := atomic.AddUint64(&id, 1)
			arg.Name = fmt.Sprintf("Velociraptor-%v", new_id)
		}

		guid, err := windows.GUIDFromString(arg.Provider)
		if err != nil {
			scope.Log("watch_etw: %s", err.Error())
			return
		}

		for {
			err = createSession(ctx, scope, guid, arg, output_chan)
			if err != nil {
				scope.Log("watch_etw: %v", err)
			}

			scope.Log("ETW session interrupted, will retry again.")
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Minute):
			}
		}

	}()

	return output_chan
}

func createSession(ctx context.Context, scope types.Scope, guid windows.GUID,
	arg *WatchETWArgs, output_chan chan vfilter.Row) error {
	session, err := etw.NewSession(guid, func(cfg *etw.SessionOptions) {
		cfg.MatchAnyKeyword = arg.AnyKeywords
		cfg.MatchAllKeyword = arg.AllKeywords
		cfg.Level = etw.TraceLevel(arg.Level)
		cfg.Name = arg.Name
	})
	if err != nil {
		scope.Log("watch_etw: %s", err.Error())
		return err
	}

	// Signal to the callback to ignore more messages
	var cancelled uint64

	sub_ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cb := func(e *etw.Event) {
		if atomic.LoadUint64(&cancelled) > 0 {
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

		scope.Log("watch_etw: Creating session %v", arg.Name)

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

	atomic.StoreUint64(&cancelled, 1)

	err = session.Close()
	if err != nil {
		return err
	}

	// Wait here for session.Process() to finish - there
	// may be a queue of events to send to the callback
	// that ETW will try to clear so we need to wait here
	// until it does.
	wg.Wait()

	return nil
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
