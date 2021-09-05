package usn

import (
	"context"
	"sync"
	"time"

	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/windows/filesystems/readers"
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
	device string,
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	output_chan chan vfilter.Row) func() {

	self.mu.Lock()
	defer self.mu.Unlock()

	device, _, err := paths.GetDeviceAndSubpath(device)
	if err != nil {
		scope.Log("watch_usn: %v", err)
		return func() {}
	}

	ntfs_ctx, err := readers.GetNTFSContext(scope, device)
	if err != nil {
		scope.Log("watch_usn: %v", err)
		return func() {}
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
	registration, pres := self.registrations[device]
	if !pres {
		registration = []*Handle{}
		self.registrations[device] = registration

		// There were no earlier registrations so launch the
		// watcher on a new handle.
		go self.StartMonitoring(config_obj, device, ntfs_ctx, frequency)
	}

	registration = append(registration, handle)
	self.registrations[device] = registration

	// De-queue the handle from the registration map when the
	// caller is done with it.
	return func() {
		self.mu.Lock()
		defer self.mu.Unlock()

		handles, pres := self.registrations[device]
		if pres {
			scope.Log("Unregistering USN log watcher for %v with handle %v",
				device, handle.id)
			new_handles := make([]*Handle, 0, len(handles))
			for _, old_handle := range handles {
				if old_handle.id != handle.id {
					new_handles = append(new_handles, old_handle)
				}
			}
			self.registrations[device] = new_handles
		}
		cancel()
	}
}

// Monitor the filename for new events and emit them to all interested
// listeners. If no listeners exist we terminate the registration.
func (self *USNWatcherService) StartMonitoring(
	config_obj *config_proto.Config,
	device string, ntfs_ctx *ntfs.NTFSContext, frequency uint64) {
	defer utils.CheckForPanic("StartMonitoring")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if frequency == 0 {
		frequency = FREQUENCY
	}

	usn_chan := ntfs.WatchUSN(ctx, ntfs_ctx, int(frequency))

	for {
		select {

		// Check every second if there are registrations. This
		// allows us to de-register with go-ntfs ASAP
		case <-time.After(time.Second):
			self.mu.Lock()
			handlers, pres := self.registrations[device]

			// No more registrations, we dont care any more.
			if !pres || len(handlers) == 0 {
				delete(self.registrations, device)
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

			self.mu.Lock()

			handlers, pres := self.registrations[device]
			if pres {
				// Distribute to all listeners
				enriched_event := makeUSNRecord(event)
				for _, handler := range handlers {
					select {
					// This handler is done -drop the event for this handler.
					case <-handler.ctx.Done():
					case handler.output_chan <- enriched_event:
					}
				}
			}
			self.mu.Unlock()
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
