package csv

import (
	"context"
	"sync"
	"time"

	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/vfilter"
)

const (
	FREQUENCY = 3 * time.Second
)

var (
	GlobalCSVService = NewCSVWatcherService()
)

// This service watches one or more many event logs files and
// multiplexes events to multiple readers.
type CSVWatcherService struct {
	mu sync.Mutex

	registrations map[string][]*Handle
}

func NewCSVWatcherService() *CSVWatcherService {
	return &CSVWatcherService{
		registrations: make(map[string][]*Handle),
	}
}

func (self *CSVWatcherService) Register(
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
func (self *CSVWatcherService) StartMonitoring(
	filename string, accessor string) {
	last_event := self.findLastEvent(filename, accessor)
	no_handlers := false

	for {
		last_event, no_handlers = self.monitorOnce(
			filename, accessor, last_event)
		if no_handlers {
			break
		}

		time.Sleep(FREQUENCY)
	}
}

func (self *CSVWatcherService) findLastEvent(
	filename string,
	accessor_name string) int {

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

	// Skip all the rows until the end.
	csv_reader := csv.NewReader(fd)
	for {
		_, err := csv_reader.ReadAny()
		if err != nil {
			break
		}
	}

	return int(csv_reader.ByteOffset)
}

func (self *CSVWatcherService) monitorOnce(
	filename string,
	accessor_name string,
	last_event int) (int, bool) {

	self.mu.Lock()
	defer self.mu.Unlock()

	key := filename + accessor_name
	handles, pres := self.registrations[key]
	if !pres {
		return 0, false
	}

	accessor, err := glob.GetAccessor(
		accessor_name, context.Background())
	if err != nil {
		return 0, false
	}

	fd, err := accessor.Open(filename)
	if err != nil {
		for _, handle := range handles {
			handle.scope.Log("Unable to open file %s: %v",
				filename, err)
		}
		return 0, false
	}
	defer fd.Close()

	csv_reader := csv.NewReader(fd)
	csv_reader.RequireLineSeperator = true

	headers, err := csv_reader.Read()
	if err != nil {
		return 0, false
	}

	// Seek to the last place we were.
	csv_reader.Seek(int64(last_event))
	for {
		row_data, err := csv_reader.ReadAny()
		if err != nil {
			return last_event, false
		}
		last_event = int(csv_reader.ByteOffset)

		row := vfilter.NewDict()
		for idx, row_item := range row_data {
			if idx > len(headers) {
				break
			}
			row.Set(headers[idx], row_item)
		}

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

			case handle.output_chan <- row:
				new_handles = append(new_handles, handle)
			}
		}

		// No more listeners - we dont care any more.
		if len(new_handles) == 0 {
			delete(self.registrations, key)
			return last_event, true
		}

		// Update the registrations - possibly
		// omitting finished listeners.
		self.registrations[key] = new_handles
		handles = new_handles
	}

	return last_event, false
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
