package syslog

import (
	"bytes"
	"context"
	"io"
	"os"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

var (
	mu             sync.Mutex
	gSyslogService *SyslogWatcherService
)

func GlobalSyslogService(config_obj *config_proto.Config) *SyslogWatcherService {
	mu.Lock()
	defer mu.Unlock()

	if gSyslogService == nil {
		gSyslogService = NewSyslogWatcherService(config_obj)
	}
	return gSyslogService
}

// This service watches one or more event logs files and multiplexes
// events to multiple readers.
type SyslogWatcherService struct {
	mu sync.Mutex

	config_obj    *config_proto.Config
	registrations map[string][]*Handle

	sleep_time  time.Duration
	buffer_size int64

	monitor_count int
}

func NewSyslogWatcherService(config_obj *config_proto.Config) *SyslogWatcherService {

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

	return &SyslogWatcherService{
		sleep_time:    sleep_time,
		buffer_size:   buffer_size,
		config_obj:    config_obj,
		registrations: make(map[string][]*Handle),
	}
}

func (self *SyslogWatcherService) Register(
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

	key := filename.String() + accessor
	registration, pres := self.registrations[key]
	if !pres {
		registration = []*Handle{}
		self.registrations[key] = registration

		go self.StartMonitoring(scope, filename, accessor)
	}

	registration = append(registration, handle)
	self.registrations[key] = registration

	scope.Log("Registering watcher for %v", filename)

	return cancel
}

// Monitor the filename for new events and emit them to all interested
// listeners. If no listeners exist we terminate.
func (self *SyslogWatcherService) StartMonitoring(
	base_scope vfilter.Scope, filename *accessors.OSPath,
	accessor_name string) {

	defer utils.CheckForPanic("StartMonitoring")

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

	cursor := self.findLastLineOffset(filename, accessor)
	key := filename.String() + accessor_name
	for {
		self.mu.Lock()
		registration, pres := self.registrations[key]
		self.mu.Unlock()

		// No more listeners left, we are done.
		if !pres || len(registration) == 0 {
			return
		}

		cursor = self.monitorOnce(filename, accessor_name, accessor, cursor)

		time.Sleep(self.sleep_time)
	}
}

func (self *SyslogWatcherService) findLastLineOffset(
	filename *accessors.OSPath,
	accessor accessors.FileSystemAccessor) *Cursor {

	cursor := &Cursor{}

	stat, err := accessor.LstatWithOSPath(filename)
	if err != nil {
		return cursor
	}

	fd, err := accessor.OpenWithOSPath(filename)
	if err != nil {
		return cursor
	}
	defer fd.Close()

	cursor.file_size = stat.Size()
	last_offset := cursor.file_size - self.buffer_size
	if last_offset < 0 {
		last_offset = 0
	}

	// File must be seekable and statable.
	_, err = fd.Seek(int64(last_offset), 0)
	if err != nil {
		return cursor
	}

	buff := make([]byte, self.buffer_size)
	n, err := fd.Read(buff)
	if err != nil && err != io.EOF {
		return cursor
	}

	buff = buff[:n]

	idx := bytes.LastIndexByte(buff, '\n')
	if idx > 0 {
		cursor.last_line_offset = last_offset + int64(idx) + 1
	}

	return cursor
}

func (self *SyslogWatcherService) Reap() {
	self.mu.Lock()
	defer self.mu.Unlock()

	new_registrations := make(map[string][]*Handle)

	for key, handles := range self.registrations {
		new_handles := make([]*Handle, 0, len(handles))
		for _, handle := range handles {
			select {
			case <-handle.ctx.Done():
				handle.scope.Log("Unregistering watcher for %v", key)
			default:
				new_handles = append(new_handles, handle)
			}
		}
		if len(new_handles) > 0 {
			new_registrations[key] = new_handles
		}
	}
	self.registrations = new_registrations
}

func (self *SyslogWatcherService) monitorOnce(
	filename *accessors.OSPath,
	accessor_name string,
	accessor accessors.FileSystemAccessor,
	cursor *Cursor) *Cursor {

	self.mu.Lock()
	defer func() {
		self.monitor_count++
		self.mu.Unlock()
	}()

	stat, err := accessor.LstatWithOSPath(filename)
	if err != nil {
		return cursor
	}

	// Nothing to do - file size is not changed since last time.
	if stat.Size() == cursor.file_size {
		return cursor
	}

	// The file is suddenly smaller than before - this could mean the
	// file was truncated.
	if stat.Size() < cursor.file_size {
		logger := logging.GetLogger(self.config_obj, &logging.ClientComponent)
		logger.Info("File size (%v) is smaller than last known size (%v) - assuming file was truncated. Will start reading at the start again.",
			stat.Size(), cursor.file_size)
		cursor.last_line_offset = 0
	}

	cursor.file_size = stat.Size()

	key := filename.String() + accessor_name
	handles, pres := self.registrations[key]
	if !pres {
		return cursor
	}

	fd, err := accessor.OpenWithOSPath(filename)
	if err != nil {
		return cursor
	}
	defer fd.Close()

	for {
		// Only read up to the last offset in case the file grows more.
		to_read := cursor.file_size - cursor.last_line_offset
		if to_read > self.buffer_size {
			to_read = self.buffer_size
		}

		if to_read == 0 {
			return cursor
		}

		// File must be seekable
		pos, err := fd.Seek(cursor.last_line_offset, os.SEEK_SET)
		if err != nil {
			return cursor
		}

		// File is truncated, start reading from the front again.
		if cursor.last_line_offset != pos {
			cursor.last_line_offset = 0
			return cursor
		}

		buff := make([]byte, to_read)
		n, err := fd.Read(buff)
		if err != nil && err != io.EOF {
			return cursor
		}

		buff = buff[:n]

		// Read whole lines inside the buffer
		for len(buff) > 0 {
			new_lf := bytes.IndexByte(buff, '\n')
			if new_lf >= 0 {
				cursor.last_line_offset += int64(new_lf) + 1

				handles = self.distributeLine(
					string(buff[:new_lf]), filename, key, handles)

				// No more listeners - we dont care any more.
				if len(handles) == 0 {
					return cursor
				}

				// Advance the cursor to the next line
				buff = buff[new_lf+1:]

				// Get next line
				continue
			}

			// The entire buffer does not contain lf at all - we skip
			// the entire buffer and hope to get a line on the next
			// read.
			if len(buff) == n {

				// The last line is just too long for the buffer. We
				// break it up into multiple lines to fit.
				if cursor.file_size-cursor.last_line_offset >
					self.buffer_size {
					cursor.last_line_offset += int64(len(buff))

					handles = self.distributeLine(
						string(buff), filename, key, handles)

					// No more listeners - we dont care any more.
					if len(handles) == 0 {
						return cursor
					}

					// Drop the buffer and read some more.
					break
				}

				return cursor
			}

			// Abandon this buffer and try again
			break
		}
	}
}

// Send the syslog line to all listeners.
func (self *SyslogWatcherService) distributeLine(
	line string,
	filename *accessors.OSPath,
	key string,
	handles []*Handle) []*Handle {
	event := ordereddict.NewDict().
		Set("OSPath", filename).
		Set("Line", line)

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
	file_size        int64
	last_line_offset int64
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
