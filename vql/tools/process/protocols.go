package process

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

// Modules are associative
type ProcessTrackerAssociative struct{}

func (self ProcessTrackerAssociative) Applicable(a types.Any, b types.Any) bool {
	_, a_ok := a.(*ProcessTracker)
	_, b_ok := b.(string)
	return a_ok && b_ok
}
func (self ProcessTrackerAssociative) GetMembers(
	scope vfilter.Scope, a vfilter.Any) []string {
	_, a_ok := a.(*ProcessTracker)
	if a_ok {
		return []string{"Processes", "GetChain", "Track"}
	}
	return nil
}

func (self ProcessTrackerAssociative) Associative(
	scope vfilter.Scope, a vfilter.Any, b vfilter.Any) (
	vfilter.Any, bool) {

	b_str, b_ok := b.(string)
	if !b_ok {
		return vfilter.Null{}, false
	}

	tracker, a_ok := a.(*ProcessTracker)
	if !a_ok {
		return vfilter.Null{}, false
	}

	switch b_str {
	case "GetChain":
		return &getChain{tracker}, true

	case "Processes":
		return tracker.Processes(), true

	case "Track":
		return &ProcessTrackerUpdater{tracker: tracker}, true

	}

	return vfilter.Null{}, false
}

// SELECTing from the process tracker will return update events when
// the state of this tracker changes. This allows another tracker to
// precisely track this tracker's state by simply replaying the update
// events.
type ProcessTrackerUpdater struct {
	tracker *ProcessTracker
}

func (self ProcessTrackerUpdater) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{}
}

func (self ProcessTrackerUpdater) Call(
	ctx context.Context, scope types.Scope,
	args *ordereddict.Dict) <-chan types.Row {

	output_chan := make(chan types.Row)

	self.tracker.mu.Lock()
	if self.tracker.update_notifications == nil {
		self.tracker.update_notifications = make(chan *ProcessEntry)
	}
	update_notifications := self.tracker.update_notifications
	self.tracker.mu.Unlock()

	go func() {
		defer close(output_chan)

		// First message is a full sync message.
		event := &ProcessEntry{
			UpdateType: "sync",
			Data:       self.tracker.Processes(),
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
