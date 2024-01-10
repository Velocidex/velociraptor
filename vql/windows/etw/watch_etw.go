//go:build windows && cgo
// +build windows,cgo

package etw

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	"golang.org/x/sys/windows"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type WatchETWArgs struct {
	Name          string          `vfilter:"optional,field=name,doc=A session name "`
	Provider      string          `vfilter:"required,field=guid,doc=A Provider GUID to watch "`
	AnyKeywords   uint64          `vfilter:"optional,field=any,doc=Any Keywords "`
	AllKeywords   uint64          `vfilter:"optional,field=all,doc=All Keywords "`
	Level         int64           `vfilter:"optional,field=level,doc=Log level (0-5)"`
	Stop          *vfilter.Lambda `vfilter:"optional,field=stop,doc=If provided we stop watching automatically when this lambda returns true"`
	Timeout       uint64          `vfilter:"optional,field=timeout,doc=If provided we stop after this much time"`
	CaptureState  bool            `vfilter:"optional,field=capture_state,doc=If true, capture the state of the provider when the event is triggered"`
	EnableMapInfo bool            `vfilter:"optional,field=enable_map_info,doc=Resolving MapInfo with TdhGetEventMapInformation is very expensive and causes events to be dropped so we disabled it by default. Enable with this flag."`
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

		if arg.Timeout > 0 {
			sub_ctx, timeout_cancel := context.WithTimeout(
				ctx, time.Duration(arg.Timeout)*time.Second)
			defer timeout_cancel()

			ctx = sub_ctx
		}

		options := ETWOptions{
			AllKeyword:    arg.AllKeywords,
			AnyKeyword:    arg.AnyKeywords,
			Level:         arg.Level,
			CaptureState:  arg.CaptureState,
			EnableMapInfo: arg.EnableMapInfo,
		}

		for i := 0; i < 10; i++ {
			err = self.WatchOnce(ctx, scope, arg.Stop, output_chan,
				arg.Name, options, wGuid)
			if err != nil {
				scope.Log("watch_etw: ETW session interrupted, will retry again in 2 minutes.")
				utils.SleepWithCtx(ctx, 2*time.Minute)
				continue
			}
			return
		}
		scope.Log("watch_etw: Repeatedly interrupted, giving up.")
	}()

	return output_chan
}

func (self WatchETWPlugin) WatchOnce(
	ctx context.Context, scope vfilter.Scope,
	stop *vfilter.Lambda, output_chan chan vfilter.Row,
	session string, options ETWOptions,
	wGuid windows.GUID) error {

	cancel, event_channel, err := GlobalEventTraceService.Register(
		ctx, scope, session, options, wGuid)
	if err != nil {
		return err
	}
	defer cancel()

	// Wait until the query is complete.
	for {
		select {
		case <-ctx.Done():
			return nil

		case event, ok := <-event_channel:
			if !ok {
				return nil
			}
			if stop != nil &&
				scope.Bool(stop.Reduce(ctx, scope, []vfilter.Any{event})) {
				scope.Log("watch_etw: Aborting query due to stop condition.")
				return nil
			}

			select {
			case <-ctx.Done():
				return nil
			case output_chan <- event:
			}
		}
	}

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
