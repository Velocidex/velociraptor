package process

import (
	"context"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

// SELECTing from the process tracker will return update events when
// the state of this tracker changes. This allows another tracker to
// precisely track this tracker's state by simply replaying the update
// events.
type ProcessTrackerUpdater struct{}

func (self ProcessTrackerUpdater) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "process_tracker_updates",
		Doc:  "Get the process tracker update events from the global process tracker.",
	}
}

func (self ProcessTrackerUpdater) Call(
	ctx context.Context, scope types.Scope,
	args *ordereddict.Dict) <-chan types.Row {

	output_chan := make(chan types.Row)

	tracker := GetGlobalTracker()
	update_notifications := tracker.Updates()

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "process_tracker_updates", args)()

		if tracker == nil {
			return
		}

		// First message is a full sync message.
		update := ordereddict.NewDict()
		for _, p := range tracker.Processes(ctx, scope) {
			update.Set(p.Id, p)
		}
		event := &UpdateProcessEntry{
			UpdateType: "sync",
			Data:       update,
		}
		select {
		case <-ctx.Done():
			return
		case output_chan <- event:
		}

		for {
			select {
			case <-ctx.Done():
				return

			case update, ok := <-update_notifications:
				if !ok {
					return
				}
				output_chan <- update
			}
		}
	}()

	return output_chan
}

func init() {
	vql_subsystem.RegisterPlugin(&ProcessTrackerUpdater{})
}
