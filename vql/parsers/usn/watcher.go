package usn

import (
	"context"
	"fmt"
	"sync"
	"time"

	"www.velocidex.com/golang/go-ntfs/parser"
	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/ntfs/readers"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

const (
	// This is the default frequency. You can override this frequency
	// by setting the VQL environment variable USN_FREQUENCY
	FREQUENCY = 30 // Seconds
)

var (
	GlobalEventLogService = NewUSNWatcherService()
)

// This service watches the USN log on one or more devices and
// forwards events to multiple readers.
type USNWatcherService struct {
	mu sync.Mutex

	// Handlers get an incrementing id.
	idx int

	// Registrations for each device
	registrations map[string][]*Handle
}

func NewUSNWatcherService() *USNWatcherService {
	return &USNWatcherService{
		registrations: make(map[string][]*Handle),
	}
}

func (self *USNWatcherService) Register(
	device *accessors.OSPath,
	accessor string,
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	output_chan chan vfilter.Row) (func(), error) {

	self.mu.Lock()
	defer self.mu.Unlock()

	ntfs_ctx, err := readers.GetNTFSContext(scope, device, accessor)
	if err != nil {
		return nil, fmt.Errorf("while opening device %v: %w", device, err)
	}

	subctx, cancel := context.WithCancel(ctx)

	// Make a new handle for this caller.
	self.idx++
	handle := &Handle{
		id:          self.idx,
		ctx:         subctx,
		output_chan: output_chan,
		scope:       scope}

	frequency := vql_subsystem.GetIntFromRow(scope, scope,
		constants.USN_FREQUENCY)
	if frequency == 0 {
		frequency = FREQUENCY
	}

	scope.Log("Registering USN log watcher for %v with handle %v and frequency %v seconds",
		device, handle.id, frequency)

	// Get existing registrations and append the new one to them.
	key := device.String()
	registration, pres := self.registrations[key]
	if !pres {
		registration = []*Handle{}
		self.registrations[key] = registration

		// There were no earlier registrations so launch the
		// watcher on a new handle.
		go self.StartMonitoring(config_obj, device, ntfs_ctx, frequency)
	}

	registration = append(registration, handle)
	self.registrations[key] = registration

	// De-queue the handle from the registration map when the
	// caller is done with it.
	return func() {
		self.mu.Lock()
		defer self.mu.Unlock()

		handles, pres := self.registrations[key]
		if pres {
			scope.Log("Unregistering USN log watcher for %v with handle %v",
				device, handle.id)
			new_handles := make([]*Handle, 0, len(handles))
			for _, old_handle := range handles {
				if old_handle.id != handle.id {
					new_handles = append(new_handles, old_handle)
				}
			}
			self.registrations[key] = new_handles
		}
		cancel()
	}, nil
}

// Distribute the event to all interested listeners.
func (self *USNWatcherService) distributeEvent(
	event *parser.USN_RECORD, key string) {

	self.mu.Lock()
	handlers, pres := self.registrations[key]

	// Make a local copy to ensure the handler can be unregistered. If
	// it does, the handler's ctx will be done so this becomes a noop.
	handlers = handlers[:]
	self.mu.Unlock()

	if pres {
		// Distribute to all listeners
		enriched_event := makeUSNRecord(event)
		for _, handler := range handlers {
			select {
			// This handler is done - drop the event for this handler.
			case <-handler.ctx.Done():
			case handler.output_chan <- enriched_event:

				// Wait up to 10 seconds to send the event
				// otherwise drop it.
			case <-time.After(10 * time.Second):
			}
		}
	}
}

// Monitor the filename for new events and emit them to all interested
// listeners. If no listeners exist we terminate the registration.
func (self *USNWatcherService) StartMonitoring(
	config_obj *config_proto.Config,
	device *accessors.OSPath, ntfs_ctx *ntfs.NTFSContext, frequency uint64) {
	defer utils.CheckForPanic("StartMonitoring")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if frequency == 0 {
		frequency = FREQUENCY
	}

	usn_chan := ntfs.WatchUSN(ctx, ntfs_ctx, int(frequency))
	key := device.String()
	for {
		select {

		// Check every second if there are registrations. This
		// allows us to de-register with go-ntfs ASAP
		case <-time.After(time.Second):
			self.mu.Lock()
			handlers, pres := self.registrations[key]

			// No more registrations, we dont care any more.
			if !pres || len(handlers) == 0 {
				delete(self.registrations, key)
				logger := logging.GetLogger(config_obj, &logging.ClientComponent)
				logger.Info("Unregistering USN log watcher for %v", device)

				self.mu.Unlock()
				return
			}
			self.mu.Unlock()

		case event, ok := <-usn_chan:
			if !ok {
				return
			}
			self.distributeEvent(event, key)
		}
	}

}

// A handle is given for each interested party. We write the event on
// to the output_chan unless the context is done. When all interested
// parties are done we may destroy the monitoring goroutine and remove
// the registration.
type Handle struct {
	ctx         context.Context
	output_chan chan vfilter.Row
	scope       vfilter.Scope
	id          int
}
