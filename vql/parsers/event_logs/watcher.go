package event_logs

import (
	"context"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/evtx"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/repository"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/vfilter"
)

var (
	GlobalEventLogService = NewEventLogWatcherService(context.Background())
)

// This service watches one or more event logs files and multiplexes
// events to multiple readers.
type EventLogWatcherService struct {
	mu sync.Mutex

	ctx           context.Context
	registrations map[string][]*Handle
}

func NewEventLogWatcherService(ctx context.Context) *EventLogWatcherService {
	return &EventLogWatcherService{
		registrations: make(map[string][]*Handle),
		ctx:           ctx,
	}
}

func (self *EventLogWatcherService) Register(
	filename *accessors.OSPath,
	accessor string,
	ctx context.Context,
	scope vfilter.Scope,
	output_chan chan vfilter.Row) func() {

	self.mu.Lock()
	defer self.mu.Unlock()

	subctx, cancel := context.WithCancel(ctx)

	handle := &Handle{
		ctx:         subctx,
		output_chan: output_chan,
		scope:       scope}

	frequency := vql_subsystem.GetIntFromRow(
		scope, scope, constants.EVTX_FREQUENCY)
	if frequency == 0 {
		frequency = 15
	}

	key := filename.String() + accessor
	registration, pres := self.registrations[key]
	if !pres {
		registration = []*Handle{}
		self.registrations[key] = registration

		// Create a scope with a completely different lifespan since
		// it may outlive this query (if another query starts watching
		// the same file). The query will inherit the same ACL
		// manager, log manager etc but this is usually fine as there
		// are not different ACLs managers on the client side.
		manager := &repository.RepositoryManager{}
		builder := services.ScopeBuilderFromScope(scope)
		subscope := manager.BuildScope(builder)

		go self.StartMonitoring(self.ctx,
			subscope, filename, accessor, frequency)
	}

	registration = append(registration, handle)
	self.registrations[key] = registration

	scope.Log("Registering watcher for %v with frequency %v", filename, frequency)

	return cancel
}

// Monitor the filename for new events and emit them to all interested
// listeners. If no listeners exist we terminate.
func (self *EventLogWatcherService) StartMonitoring(
	ctx context.Context,
	scope vfilter.Scope,
	filename *accessors.OSPath,
	accessor_name string, frequency uint64) {
	defer scope.Close()

	defer utils.CheckForPanic("StartMonitoring")

	// By default check every 15 seconds. Event logs are not flushed
	// that often so checking more frequently does not help much.
	if frequency == 0 {
		frequency = 15
	}

	// A resolver for messages
	resolver, _ := evtx.GetNativeResolver()
	accessor, err := accessors.GetAccessor(accessor_name, scope)
	if err != nil {
		scope.Log("Registering watcher error: %v", err)
		return
	}

	last_event := self.findLastEvent(scope, filename, accessor_name, accessor)
	key := filename.String() + accessor_name
	for {
		self.mu.Lock()
		registration, pres := self.registrations[key]
		self.mu.Unlock()

		// No more listeners left, we are done.
		if !pres || len(registration) == 0 {
			return
		}

		last_event = self.monitorOnce(
			filename, accessor_name, accessor, last_event, resolver)

		duration := utils.Jitter(time.Duration(frequency) * time.Second)

		eventLogWatchTracker.SetNextScan(
			filename, accessor_name, utils.GetTime().Now().Add(duration))

		utils.SleepWithCtx(ctx, duration)
	}
}

func (self *EventLogWatcherService) findLastEvent(
	scope vfilter.Scope,
	filename *accessors.OSPath,
	accessor_name string,
	accessor accessors.FileSystemAccessor) int {

	defer eventLogWatchTracker.ChargeFindLastEvent(filename, accessor_name)()

	last_event := 0

	fd, err := accessor.OpenWithOSPath(filename)
	if err != nil {
		scope.Log("findLastEvent Open error: %v", err)
		return 0
	}
	defer fd.Close()

	chunks, err := evtx.GetChunks(fd)
	if err != nil {
		scope.Log("findLastEvent GetChunks error: %v", err)
		return 0
	}
	for _, c := range chunks {
		if c == nil {
			continue
		}

		if int(c.Header.LastEventRecID) > last_event {
			last_event = int(c.Header.LastEventRecID)
		}

	}
	return last_event
}

func (self *EventLogWatcherService) getActiveHandles(key string) []*Handle {
	handles, pres := self.registrations[key]
	if !pres {
		return nil
	}

	new_handles := make([]*Handle, 0, len(handles))
	for _, h := range handles {
		select {
		case <-h.ctx.Done():
			continue
		default:
			new_handles = append(new_handles, h)
		}
	}

	if len(new_handles) == 0 {
		delete(self.registrations, key)
	}

	return new_handles
}

func (self *EventLogWatcherService) monitorOnce(
	filename *accessors.OSPath,
	accessor_name string,
	accessor accessors.FileSystemAccessor,
	last_event int,
	resolver evtx.MessageResolver) int {

	defer eventLogWatchTracker.ChargeMonitorOnce(filename, accessor_name)()

	self.mu.Lock()
	defer self.mu.Unlock()

	key := filename.String() + accessor_name
	handles := self.getActiveHandles(key)
	if len(handles) == 0 {
		return 0
	}

	fd, err := accessor.OpenWithOSPath(filename)
	if err != nil {
		for _, handle := range handles {
			functions.DeduplicatedLog(
				context.Background(), handle.scope,
				"Unable to open file %v: %v", filename, err)
		}
		return 0
	}
	defer fd.Close()

	chunks, err := evtx.GetChunks(fd)
	if err != nil {
		return 0
	}

	new_last_event := last_event
	for _, c := range chunks {
		if int(c.Header.LastEventRecID) <= last_event {
			continue
		}

		records, _ := c.Parse(int(last_event))
		for _, record := range records {
			event_id := int(record.Header.RecordID)
			if event_id > new_last_event {
				new_last_event = event_id
			}
			event_map, ok := record.Event.(*ordereddict.Dict)
			if !ok {
				continue
			}

			event, pres := ordereddict.GetMap(event_map, "Event")
			if !pres {
				continue
			}

			// Possibly enrich the event.
			if resolver != nil {
				event.Set("Message", evtx.ExpandMessage(event, resolver))
			}

			eventLogWatchTracker.AddRow(filename, accessor_name)

			new_handles := make([]*Handle, 0, len(handles))
			for _, handle := range handles {
				select {
				case <-handle.ctx.Done():
					// If context is done, drop the event.

				case handle.output_chan <- event:
					new_handles = append(new_handles, handle)
				}
			}

			// No more listeners - we dont care any more.
			if len(new_handles) == 0 {
				delete(self.registrations, key)
				return new_last_event
			}

			// Update the registrations - possibly
			// omitting finished listeners.
			self.registrations[key] = new_handles
			handles = new_handles
		}
	}

	return new_last_event
}

// A handle is given for each interested party. We write the event on
// to the output_chan unless the context is done. When all interested
// parties are done we may destroy the monitoring go routine and remove
// the registration.
type Handle struct {
	ctx         context.Context
	output_chan chan vfilter.Row
	scope       vfilter.Scope
}
