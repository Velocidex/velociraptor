package usn

import (
	"context"
	"sync"

	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/parsers"
	"www.velocidex.com/golang/vfilter"
)

const (
	FREQUENCY = 3 // Seconds
)

var (
	GlobalEventLogService = NewUSNWatcherService()
)

// This service watches the USN log on one or more devices and
// forwards events to multiple readers.
type USNWatcherService struct {
	mu sync.Mutex

	// Registrations for each device
	registrations map[string][]*Handle
}

func NewUSNWatcherService() *USNWatcherService {
	return &USNWatcherService{
		registrations: make(map[string][]*Handle),
	}
}

func (self *USNWatcherService) Register(
	device string,
	ctx context.Context,
	scope *vfilter.Scope,
	output_chan chan vfilter.Row) func() {

	self.mu.Lock()
	defer self.mu.Unlock()

	device, _, err := paths.GetDeviceAndSubpath(device)
	if err != nil {
		scope.Log("watch_usn: %v", err)
		return func() {}
	}

	ntfs_ctx, err := parsers.GetNTFSContext(scope, device)
	if err != nil {
		scope.Log("watch_usn: %v", err)
		return func() {}
	}

	subctx, cancel := context.WithCancel(ctx)

	handle := &Handle{
		ctx:         subctx,
		output_chan: output_chan,
		scope:       scope}

	registration, pres := self.registrations[device]
	if !pres {
		registration = []*Handle{}
		self.registrations[device] = registration

		go self.StartMonitoring(device, ntfs_ctx)
	}

	registration = append(registration, handle)
	self.registrations[device] = registration

	scope.Log("Registering USN log watcher for %v", device)

	return cancel
}

// Monitor the filename for new events and emit them to all interested
// listeners. If no listeners exist we terminate.
func (self *USNWatcherService) StartMonitoring(device string, ntfs_ctx *ntfs.NTFSContext) {
	defer utils.CheckForPanic("StartMonitoring")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for event := range ntfs.WatchUSN(ctx, ntfs_ctx, FREQUENCY) {
		handlers, pres := self.registrations[device]

		// No more registrations, we dont care any more.
		if !pres || len(handlers) == 0 {
			delete(self.registrations, device)
			return
		}

		enriched_event := makeUSNRecord(event)
		new_handles := make([]*Handle, 0, len(handlers))
		for _, handler := range handlers {
			select {
			case <-handler.ctx.Done():
				// This handler is done -drop the event.
			case handler.output_chan <- enriched_event:
				new_handles = append(new_handles, handler)
			}
		}

		// Update the registrations - possibly omitting
		// finished listeners.
		self.registrations[device] = new_handles

	}

}

// A handle is given for each interested party. We write the event on
// to the output_chan unless the context is done. When all interested
// parties are done we may destroy the monitoring goroutine and remove
// the registration.
type Handle struct {
	ctx         context.Context
	output_chan chan vfilter.Row
	scope       *vfilter.Scope
}
