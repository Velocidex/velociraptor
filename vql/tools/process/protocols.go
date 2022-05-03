package process

import (
	"context"

	"github.com/Velocidex/ordereddict"
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

	tracker.mu.Lock()
	if tracker.update_notifications == nil {
		tracker.update_notifications = make(chan *ProcessEntry)
	}
	update_notifications := tracker.update_notifications
	tracker.mu.Unlock()

	go func() {
		defer close(output_chan)

		// First message is a full sync message.
		event := &ProcessEntry{
			UpdateType: "sync",
			Data:       tracker.Processes(),
		}
		output_chan <- event

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
