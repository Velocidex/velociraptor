package syslog

import (
	"bytes"
	"context"
	"io"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

const (
	FREQUENCY   = 3 * time.Second
	BUFFER_SIZE = 16 * 1024
)

var (
	GlobalSyslogService = NewSyslogWatcherService()
)

// This service watches one or more event logs files and multiplexes
// events to multiple readers.
type SyslogWatcherService struct {
	mu sync.Mutex

	registrations map[string][]*Handle
}

func NewSyslogWatcherService() *SyslogWatcherService {
	return &SyslogWatcherService{
		registrations: make(map[string][]*Handle),
	}
}

func (self *SyslogWatcherService) Register(
	filename string,
	accessor string,
	ctx context.Context,
	scope *vfilter.Scope,
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	accessor, err := glob.GetAccessor(accessor_name, ctx)
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
	accessor glob.FileSystemAccessor) *Cursor {

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
	accessor glob.FileSystemAccessor,
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

	key := filename + accessor_name
	handles, pres := self.registrations[key]
	if !pres {
		return &Cursor{}
	}

	fd, err := accessor.Open(filename)
	if err != nil {
		return &Cursor{}
	}
	defer fd.Close()

	pos, err := fd.Seek(cursor.last_line_offset, 0)
	if err != nil {
		return &Cursor{}
	}

	// File is truncated, start reading from the front again.
	if cursor.last_line_offset != pos {
		cursor = &Cursor{}
	}

	buff := make([]byte, BUFFER_SIZE)
	n, err := fd.Read(buff)
	if err != nil && err != io.EOF {
		return &Cursor{}
	}

	buff = buff[:n]

	// Read whole lines inside the buffer
	for len(buff) > 0 {
		new_lf := bytes.IndexByte(buff, '\n')
		if new_lf > 0 {
			new_handles := self.distributeLine(
				string(buff[:new_lf]), filename, key, handles)

			// No more listeners - we dont care any more.
			if len(new_handles) == 0 {
				delete(self.registrations, key)
				return cursor
			}

			// Update the registrations - possibly
			// omitting finished listeners.
			self.registrations[key] = new_handles
			handles = new_handles

			cursor.last_line_offset += int64(new_lf) + 1
			buff = buff[new_lf+1:]

		} else {
			// The buffer does not contain lf at all - we
			// skip the entire buffer and hope to get a
			// line on the next read.
			if cursor.last_line_offset+BUFFER_SIZE < cursor.file_size {
				cursor.last_line_offset = cursor.file_size
			}

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
	scope       *vfilter.Scope
}
