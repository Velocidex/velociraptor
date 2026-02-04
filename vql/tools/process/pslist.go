package process

import (
	"context"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

type _ProcessTrackerPsList struct{}

func (self _ProcessTrackerPsList) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "process_tracker_pslist",
		Doc:  "List all processes from the process tracker.",
	}
}

func (self _ProcessTrackerPsList) Call(
	ctx context.Context, scope types.Scope,
	args *ordereddict.Dict) <-chan types.Row {

	output_chan := make(chan types.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "process_tracker_pslist", args)()

		for _, proc := range GetGlobalTracker().Processes(ctx, scope) {
			select {
			case <-ctx.Done():
				return

			case output_chan <- proc.Data():
			}
		}
	}()

	return output_chan
}

func init() {
	vql_subsystem.RegisterPlugin(&_ProcessTrackerPsList{})
}
