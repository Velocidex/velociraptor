package event_logs

import (
	"context"
	"sync"
	"time"

	"www.velocidex.com/golang/evtx"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/vfilter"
)

const (
	FREQUENCY = 3 * time.Second
)

var (
	GlobalEventLogService = NewEventLogWatcherService()
)

// This service watches one or more many event logs files and
// multiplexes events to multiple readers.
type EventLogWatcherService struct {
	mu sync.Mutex

	registrations map[string][]*Handle
}

func NewEventLogWatcherService() *EventLogWatcherService {
	return &EventLogWatcherService{
		registrations: make(map[string][]*Handle),
	}
}

func (self *EventLogWatcherService) Register(
	filename string,
	accessor string,
	ctx context.Context,
	scope *vfilter.Scope,
	output_chan chan vfilter.Row) {

	self.mu.Lock()
	defer self.mu.Unlock()

	handle := &Handle{
		ctx:         ctx,
		output_chan: output_chan,
		scope:       scope}

	key := filename + accessor
	registration, pres := self.registrations[key]
	if !pres {
		registration = []*Handle{}
		self.registrations[key] = registration
		go self.StartMonitoring(filename, accessor)
	}

	registration = append(registration, handle)
	self.registrations[key] = registration

	scope.Log("Registering watcher for %v", filename)
}

// Monitor the filename for new events and emit them to all interested
// listeners. If no listeners exist we terminate.
func (self *EventLogWatcherService) StartMonitoring(
	filename string, accessor string) {
	last_event := self.findLastEvent(filename, accessor)
	key := filename + accessor
	for {
		self.mu.Lock()
		registration, pres := self.registrations[key]
		self.mu.Unlock()

		// No more listeners left, we are done.
		if !pres || len(registration) == 0 {
			return
		}

		last_event = self.monitorOnce(
			filename, accessor, last_event)

		time.Sleep(FREQUENCY)
	}
}

func (self *EventLogWatcherService) findLastEvent(
	filename string,
	accessor_name string) int {
	last_event := 0

	accessor, err := glob.GetAccessor(
		accessor_name, context.Background())
	if err != nil {
		return 0
	}

	fd, err := accessor.Open(filename)
	if err != nil {
		return 0
	}
	defer fd.Close()

	chunks, err := evtx.GetChunks(fd)
	if err != nil {
		return 0
	}

	for _, c := range chunks {
		if int(c.Header.LastEventRecID) <= last_event {
			continue
		}

		records, _ := c.Parse(int(last_event))
		for _, record := range records {
			if int(record.Header.RecordID) > last_event {
				last_event = int(record.Header.RecordID)
			}
		}
	}

	return last_event
}

func (self *EventLogWatcherService) monitorOnce(
	filename string,
	accessor_name string,
	last_event int) int {

	self.mu.Lock()
	defer self.mu.Unlock()

	key := filename + accessor_name
	handles, pres := self.registrations[key]
	if !pres {
		return 0
	}

	accessor, err := glob.GetAccessor(
		accessor_name, context.Background())
	if err != nil {
		return 0
	}

	fd, err := accessor.Open(filename)
	if err != nil {
		for _, handle := range handles {
			handle.scope.Log("Unable to open file %s: %v",
				filename, err)
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
			event_map, ok := record.Event.(map[string]interface{})
			if ok {
				event := event_map["Event"]

				new_handles := make([]*Handle, 0, len(handles))
				for _, handle := range handles {
					select {
					case <-handle.ctx.Done():
						// Remove and close
						// handles that are
						// not currently
						// active.
						handle.scope.Log(
							"Removing watcher for %v",
							filename)
						close(handle.output_chan)

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
	}

	return new_last_event
}

// A handle is given for each interested party. We write the event on
// to the output_chan unless the context is done. When all interested
// party are done we may destroy the monitoring go routine and remove
// the registration.
type Handle struct {
	ctx         context.Context
	output_chan chan vfilter.Row
	scope       *vfilter.Scope
}
