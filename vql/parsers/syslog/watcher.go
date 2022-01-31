package syslog

import (
	"bytes"
	"context"
	"io"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

const (
	FREQUENCY   = 3 * time.Second
	BUFFER_SIZE = 16 * 1024
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
}

func NewSyslogWatcherService(config_obj *config_proto.Config) *SyslogWatcherService {
	return &SyslogWatcherService{
		config_obj:    config_obj,
		registrations: make(map[string][]*Handle),
	}
}

func (self *SyslogWatcherService) Register(
	filename string,
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

	return cancel
}

// Monitor the filename for new events and emit them to all interested
// listeners. If no listeners exist we terminate.
func (self *SyslogWatcherService) StartMonitoring(
	filename string, accessor_name string) {

	defer utils.CheckForPanic("StartMonitoring")

	scope := vql_subsystem.MakeScope()
	defer scope.Close()

	accessor, err := accessors.GetAccessor(accessor_name, scope)
	if err != nil {
		//scope.Log("Registering watcher error: %v", err)
		return
	}

	cursor := self.findLastLineOffset(filename, accessor)
	key := filename + accessor_name
	for {
		self.mu.Lock()
		registration, pres := self.registrations[key]
		self.mu.Unlock()

		// No more listeners left, we are done.
		if !pres || len(registration) == 0 {
			return
		}

		cursor = self.monitorOnce(filename, accessor_name, accessor, cursor)

		time.Sleep(FREQUENCY)
	}
}

func (self *SyslogWatcherService) findLastLineOffset(
	filename string,
	accessor accessors.FileSystemAccessor) *Cursor {

	cursor := &Cursor{}

	stat, err := accessor.Lstat(filename)
	if err != nil {
		return cursor
	}

	fd, err := accessor.Open(filename)
	if err != nil {
		return cursor
	}
	defer fd.Close()

	cursor.file_size = stat.Size()
	last_offset := cursor.file_size - BUFFER_SIZE
	if last_offset < 0 {
		last_offset = 0
	}

	// File must be seekable and statable.
	_, err = fd.Seek(int64(last_offset), 0)
	if err != nil {
		return cursor
	}

	buff := make([]byte, BUFFER_SIZE)
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

func (self *SyslogWatcherService) monitorOnce(
	filename string,
	accessor_name string,
	accessor accessors.FileSystemAccessor,
	cursor *Cursor) *Cursor {

	self.mu.Lock()
	defer self.mu.Unlock()

	stat, err := accessor.Lstat(filename)
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
		logger.Info("File size (%v) is smaller than last know size (%v) - assuming file was truncated. Will start reading at the start again.",
			stat.Size(), cursor.file_size)
		cursor.last_line_offset = 0
	}

	cursor.file_size = stat.Size()

	key := filename + accessor_name
	handles, pres := self.registrations[key]
	if !pres {
		return cursor
	}

	fd, err := accessor.Open(filename)
	if err != nil {
		return cursor
	}
	defer fd.Close()

	// File must be seekable
	pos, err := fd.Seek(cursor.last_line_offset, 0)
	if err != nil {
		return cursor
	}

	// File is truncated, start reading from the front again.
	if cursor.last_line_offset != pos {
		cursor.last_line_offset = 0
		return cursor
	}

	buff := make([]byte, BUFFER_SIZE)
	n, err := fd.Read(buff)
	if err != nil && err != io.EOF {
		return cursor
	}

	buff = buff[:n]
	offset := 0

	// Read whole lines inside the buffer
	for len(buff)-offset > 0 {
		new_lf := bytes.IndexByte(buff[offset:], '\n')
		if new_lf > 0 {
			new_handles := self.distributeLine(
				string(buff[offset:offset+new_lf]), filename, key, handles)

			// No more listeners - we dont care any more.
			if len(new_handles) == 0 {
				delete(self.registrations, key)
				cursor.last_line_offset += int64(new_lf) + 1
				return cursor
			}

			// Update the registrations - possibly
			// omitting finished listeners.
			self.registrations[key] = new_handles
			handles = new_handles

			cursor.last_line_offset += int64(new_lf) + 1
			offset += new_lf + 1

		} else {
			// The buffer does not contain lf at all - we
			// skip the entire buffer and hope to get a
			// line on the next read.
			cursor.last_line_offset += int64(len(buff) - offset)
			return cursor
		}
	}
	return cursor
}

// Send the syslog line to all listeners.
func (self *SyslogWatcherService) distributeLine(
	line, filename, key string,
	handles []*Handle) []*Handle {
	event := ordereddict.NewDict().Set("Line", line)

	new_handles := make([]*Handle, 0, len(handles))
	for _, handle := range handles {
		select {
		case <-handle.ctx.Done():
			// If context is done, drop the event.

		case handle.output_chan <- event:
			new_handles = append(new_handles, handle)
		}
	}

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
