//go:build linux && (arm64 || amd64)
// +build linux
// +build arm64 amd64

package ebpf

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/tracee_velociraptor/userspace/ebpf"
	"github.com/Velocidex/tracee_velociraptor/userspace/events"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

var (
	gEbpfManager *ebpf.EBPFManager
)

type EBPFEventPluginArgs struct {
	EventNames []string `vfilter:"required,field=events,doc=A list of event names to acquire."`
	IncludeEnv bool     `vfilter:"optional,field=include_env,doc=Include process environment variables."`
}

type EBPFEventPlugin struct{}

func (self EBPFEventPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "watch_ebpf",
		Doc:      "Watch for events from eBPF.",
		ArgType:  type_map.AddType(scope, &EBPFEventPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	}
}

func (self EBPFEventPlugin) Call(
	ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {

	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor("watch_ebpf", args)()

		err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
		if err != nil {
			scope.Log("watch_ebpf: %s", err)
			return
		}

		arg := &EBPFEventPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("watch_ebpf: %s", err.Error())
			return
		}

		var selected_events []events.ID

		for _, event_name := range arg.EventNames {
			desc, pres := ebpf.DescByEventName(event_name)
			if !pres {
				scope.Error("watch_ebpf: invalid event name %v", event_name)
				continue
			}
			id, pres := desc.GetInt64("Id")
			if pres {
				selected_events = append(selected_events, events.ID(id))
			}
		}

		if len(selected_events) == 0 {
			scope.Error("watch_ebpf: no events to watch")
			return
		}

		if gEbpfManager == nil {
			config_obj, _ := vql_subsystem.GetServerConfig(scope)
			logger := NewLogger(config_obj)
			logger.SetScope(scope)

			config := ebpf.Config{
				Options: ebpf.OptTranslateFDFilePath,
			}

			if arg.IncludeEnv {
				config.Options |= ebpf.OptExecEnv | ebpf.OptTranslateFDFilePath
			}

			gEbpfManager, err = ebpf.NewEBPFManager(
				context.Background(), config, logger)
			if err != nil {
				scope.Log("watch_ebpf: %v", err)
				return
			}

		}

		events_chan, closer, err := gEbpfManager.Watch(ctx, selected_events)
		if err != nil {
			scope.Log("watch_ebpf: %v", err)
			return
		}
		defer closer()

		for {
			select {
			case <-ctx.Done():
				return

			case row := <-events_chan:
				select {
				case <-ctx.Done():
					return

				case output_chan <- enrich(row):
				}
			}
		}
	}()

	return output_chan
}

type EBPFEventListPlugin struct{}

func (self EBPFEventListPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "ebpf_events",
		Doc:  "Dump information about potential ebpf_events that can be used by the watch_ebpf() plugin",
	}
}

func (self EBPFEventListPlugin) Call(
	ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {

	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor("watch_ebpf", args)()

		events := ebpf.GetEvents()
		for _, event := range events.Keys() {
			value, _ := events.Get(event)

			select {
			case <-ctx.Done():
				return

			case output_chan <- ordereddict.NewDict().
				Set("Event", event).
				Set("Metadata", value):
			}
		}

	}()
	return output_chan
}

func init() {
	vql_subsystem.RegisterPlugin(&EBPFEventListPlugin{})
	vql_subsystem.RegisterPlugin(&EBPFEventPlugin{})
}
