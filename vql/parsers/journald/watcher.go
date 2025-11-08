package journald

import (
	"context"
	"sync"
	"time"

	"github.com/Velocidex/go-journalctl/parser"
	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/readers"
	"www.velocidex.com/golang/vfilter"
)

var (
	mu               sync.Mutex
	gJournaldService *JournaldWatcherService
)

func StartGlobalJournaldService(
	ctx context.Context, config_obj *config_proto.Config) {
	mu.Lock()
	defer mu.Unlock()

	gJournaldService = NewJournaldWatcherService(ctx, config_obj)
}

// This service watches one or more event logs files and multiplexes
// events to multiple readers.
type JournaldWatcherService struct {
	mu sync.Mutex

	config_obj    *config_proto.Config
	registrations map[string][]*Handle

	sleep_time  time.Duration
	buffer_size int64

	monitor_count int
	ctx           context.Context
}

func NewJournaldWatcherService(
	ctx context.Context,
	config_obj *config_proto.Config) *JournaldWatcherService {

	sleep_time := 3 * time.Second
	buffer_size := int64(16 * 1024)
	if config_obj.Defaults != nil {
		if config_obj.Defaults.WatchPluginFrequency > 0 {
			sleep_time = time.Second * time.Duration(
				config_obj.Defaults.WatchPluginFrequency)
		}
		if config_obj.Defaults.WatchPluginBufferSize > 0 {
			buffer_size = config_obj.Defaults.WatchPluginBufferSize
		}
	}

	return &JournaldWatcherService{
		ctx:           ctx,
		sleep_time:    sleep_time,
		buffer_size:   buffer_size,
		config_obj:    config_obj,
		registrations: make(map[string][]*Handle),
	}
}

func (self *JournaldWatcherService) Register(
	filename *accessors.OSPath,
	accessor string,
	ctx context.Context,
	scope vfilter.Scope,
	raw bool,
	output_chan chan vfilter.Row) func() {

	self.mu.Lock()
	defer self.mu.Unlock()

	subctx, cancel := context.WithCancel(ctx)

	handle := &Handle{
		ctx:         subctx,
		output_chan: output_chan,
		scope:       scope}

	key := getKey(filename, accessor, raw)
	registration, pres := self.registrations[key]
	if !pres {
		registration = []*Handle{}
		self.registrations[key] = registration

		go self.StartMonitoring(scope, filename, accessor, raw)
	}

	registration = append(registration, handle)
	self.registrations[key] = registration

	scope.Log("Registering journald watcher for %v", filename)

	return cancel
}

// Monitor the filename for new events and emit them to all interested
// listeners. If no listeners exist we terminate.
func (self *JournaldWatcherService) StartMonitoring(
	base_scope vfilter.Scope,
	filename *accessors.OSPath,
	accessor_name string,
	raw bool) {

	defer utils.CheckForPanic("JournaldWatcherService.StartMonitoring")

	manager, err := services.GetRepositoryManager(self.config_obj)
	if err != nil {
		return
	}

	// Build a new scope with totally different lifetime than the
	// watching scope so we can outlast them. We still want things
	// like ACL managers etc though.
	builder := services.ScopeBuilderFromScope(base_scope)
	scope := manager.BuildScope(builder)
	defer scope.Close()

	accessor, err := accessors.GetAccessor(accessor_name, scope)
	if err != nil {
		scope.Log("Registering watcher error: %v", err)
		return
	}

	cursor := self.findLastSequence(filename, accessor)
	key := getKey(filename, accessor_name, raw)
	for {
		self.mu.Lock()
		registration, pres := self.registrations[key]
		self.mu.Unlock()

		// No more listeners left, we are done.
		if !pres || len(registration) == 0 {
			return
		}

		cursor = self.monitorOnce(scope, filename, accessor_name, accessor, raw, cursor)

		time.Sleep(self.sleep_time)
	}
}

func (self *JournaldWatcherService) findLastSequence(
	filename *accessors.OSPath,
	accessor accessors.FileSystemAccessor) *Cursor {

	cursor := &Cursor{}

	fd, err := accessor.OpenWithOSPath(filename)
	if err != nil {
		return cursor
	}
	defer fd.Close()

	journal, err := parser.OpenFile(utils.MakeReaderAtter(fd))
	if err == nil {
		cursor.last_seq = journal.GetLastSequence()
	}
	return cursor
}

func (self *JournaldWatcherService) monitorOnce(
	scope vfilter.Scope,
	filename *accessors.OSPath,
	accessor_name string,
	accessor accessors.FileSystemAccessor,
	raw bool,
	cursor *Cursor) *Cursor {

	self.mu.Lock()
	defer func() {
		self.monitor_count++
		self.mu.Unlock()
	}()

	reader, err := readers.NewAccessorReader(scope, accessor_name, filename, 10000)
	if err != nil {
		return cursor
	}
	defer reader.Close()

	journal, err := parser.OpenFile(reader)
	if err != nil {
		return cursor
	}

	// Parse raw logs
	journal.RawLogs = raw

	last_seq := journal.GetLastSequence()
	if last_seq == cursor.last_seq {
		return cursor
	}

	// Now we read from the old last_seq to the new last_seq
	journal.MinSeq = cursor.last_seq

	// Next run update the last_seq in the curser
	defer func() {
		cursor.last_seq = last_seq
	}()

	key := getKey(filename, accessor_name, raw)
	handles, pres := self.registrations[key]
	if !pres {
		return cursor
	}

	for log := range journal.GetLogs(self.ctx) {
		handles = self.distributeLog(log, key, handles)

		// No more listeners - we dont care any more.
		if len(handles) == 0 {
			break
		}
	}
	return cursor
}

// Send the syslog line to all listeners.
func (self *JournaldWatcherService) distributeLog(
	event *ordereddict.Dict,
	key string,
	handles []*Handle) []*Handle {

	new_handles := make([]*Handle, 0, len(handles))
	for _, handle := range handles {
		select {
		case <-handle.ctx.Done():
			// If context is done, drop the event.

		case handle.output_chan <- event:
			new_handles = append(new_handles, handle)
		}
	}

	// Update the registrations - possibly omitting finished
	// listeners.
	if len(new_handles) == 0 {
		delete(self.registrations, key)
	}
	self.registrations[key] = new_handles

	return new_handles
}

type Cursor struct {
	last_seq uint64
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

func getKey(filename *accessors.OSPath, accessor string, raw bool) string {
	res := filename.String() + accessor
	if raw {
		res += "_raw"
	}
	return res
}
