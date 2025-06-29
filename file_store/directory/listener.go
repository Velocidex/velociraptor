package directory

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

// A listener wraps a channel that our client will listen on. The
// client will remove events from the channel in its own time and will
// block waiting for new messages. The sender will send a message to
// the Listener object using Listener.Send(). If a receiver is ready
// to receive that message it will be delivered immediately. If no
// receivers are immediately available, the message will be sent to
// the file ring buffer.

// In order to preserve message ordering, as soon as a message is
// diverted to the buffer files, any new messages will not be directly
// delivered but will be enqueued. This guarantees that message
// ordering is preserved. When the file buffer is fully drained, the
// Listener is able to go back into direct delivering mode.
type Listener struct {
	// If set do not use the file buffer - this will block senders!
	disable_file_buffering int32

	mu sync.Mutex

	start time.Time

	// should new messages go directly to the file buffer?
	file_buffer_active bool // Locked

	// If we are closed we drop any new messages.
	closed bool

	id uint64

	name    string
	options api.QueueOptions

	// The consumer interested in these events. The consumer may
	// block arbitrarily.
	output chan *ordereddict.Dict

	// A backup file to store extra messages.
	file_buffer *FileBasedRingBuffer

	// Channel to signal that we should start pumping events from the
	// file buffer to the output.
	file_buffer_ready chan bool

	// Listener context.
	ctx    context.Context
	cancel func()

	// Number of events forwarded to this listener.
	count int64
}

func (self *Listener) Stats() *ordereddict.Dict {
	self.mu.Lock()
	defer self.mu.Unlock()

	file_buffer := ""
	if self.file_buffer_active {
		file_buffer = self.file_buffer.GetBackingFile()
	}

	return ordereddict.NewDict().
		Set("Name", self.name).
		Set("Started", self.start).
		Set("Options", self.options).
		Set("FileBuffer", file_buffer).
		Set("EventCount", self.count)
}

// Should not block - very fast.
func (self *Listener) Send(item *ordereddict.Dict) {
	defer utils.CheckForPanic("Listener.Send: %v", item)

	if self.closed {
		return
	}

	// This will block senders until we can send output
	if atomic.LoadInt32(&self.disable_file_buffering) > 0 {
		select {
		case <-self.ctx.Done():
			return

			// Try to deliver message immediately.
		case self.output <- item:
			self.mu.Lock()
			self.count++
			self.mu.Unlock()
			return
		}
	}

	self.mu.Lock()
	defer self.mu.Unlock()

	// Ignore events sent to a closed listener.
	if self.closed {
		return
	}

	// No direct delivery available - force buffer file enqueue
	if self.file_buffer_active {
		_ = self.file_buffer.Enqueue(item)
		return
	}

	select {
	case <-self.ctx.Done():
		return

		// Try to deliver message immediately.
	case self.output <- item:

		// Otherwise push to the file - this switches to Buffer File
		// enqueue mode, all further messages will be enqueued to the
		// file until it is drained.
	default:
		_ = self.file_buffer.Enqueue(item)

		// Switch to file buffer mode
		self._switchToFileMode()
	}
}

func (self *Listener) _file_buffer_ready() chan bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.file_buffer_ready
}

func (self *Listener) _switchToFileMode() {
	if !self.file_buffer_active {
		// Moving to file buffer mode will put new events in the
		// file buffer directly.
		self.file_buffer_active = true

		// Closing the sync channel will start feeding events from
		// the file to the output.
		close(self.file_buffer_ready)
	}
}

// Switch from file mode to direct mode. If any messages are in the
// buffer we drain them too.
func (self *Listener) _switchToDirectMode() {
	if self.file_buffer_active {
		// We can start sending messages directly
		self.file_buffer_active = false

		// Automatic draining of messages no longer needed.
		self.file_buffer_ready = make(chan bool)
	}
}

// A Channel readers can read events from.
func (self *Listener) Output() chan *ordereddict.Dict {
	return self.output
}

// When Close is called, we:
// 1. Stop receiving new messages.
// 2. Drain all messages from the file buffer into the output (this may block).
// 3. Close the output to release readers downstream.
func (self *Listener) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Stop new messages from coming in.
	self.closed = true

	// Stop the file buffer pump
	self._switchToDirectMode()

	// Wait for all outstanding file buffer messages to be sent.
	if self.file_buffer != nil {
		self.file_buffer.Wg.Wait()

		// Drain the file one last time.
		items := self.file_buffer.Lease(100)
		for _, item := range items {
			select {
			case <-self.ctx.Done():
				// Just drop all work items on the floor
				self.file_buffer.Wg.Done()

				// As each message is delivered we can let the
				// file buffer know it is delivered.
			case self.output <- item:
				self.file_buffer.Wg.Done()
			}
		}
		self.file_buffer.Wg.Wait()
		self.file_buffer.Close()
	}

	// Close the output to release our readers.
	close(self.output)

	self.cancel()
}

func (self *Listener) FileBufferSize() int64 {
	return self.file_buffer.PendingSize()
}

func (self *Listener) Debug() *ordereddict.Dict {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := ordereddict.NewDict().
		Set("Name", self.name).
		Set("file_buffer_active", self.file_buffer_active).
		Set("closed", self.closed).
		Set("owner", self.options.OwnerName)

	if self.file_buffer != nil {
		result.Set("PendingSize", self.file_buffer.PendingSize())
		backing_file := self.file_buffer.GetBackingFile()
		if backing_file != "" {
			result.Set("BackingFile", backing_file)
			st, err := os.Stat(backing_file)
			if err == nil {
				result.Set("Size", int64(st.Size()))
			}
		}
	}

	return result
}

func (self *Listener) pumpFileBufferToOutput(ctx context.Context) {
	for {
		// Wait here until the file has some data in it.
		select {
		case <-ctx.Done():
			return

			// Wait here until the Send() routine signals that
			// messages were enqueued.
		case <-self._file_buffer_ready():
			// Get some messages from the buffer file.
			lease_size := self.options.FileBufferLeaseSize
			if lease_size == 0 {
				lease_size = 100
			}

			self.mu.Lock()
			items := self.file_buffer.Lease(lease_size)
			if len(items) == 0 {
				// Buffer file is empty - reset the trigger and
				// signal to the Send() function that direct
				// delivery is allowed again.
				self._switchToDirectMode()
			}
			self.mu.Unlock()

			// Try to deliver messages - this can take a while but
			// we no longer hold the lock, so Send() can continue
			// enqueuing messages to the file.
			for _, item := range items {
				select {
				case <-self.ctx.Done():
					// Just drain all work items so we can safely exit
					self.file_buffer.Wg.Done()

					// As each message is delivered we can let the
					// file buffer know it is delivered.
				case self.output <- item:
					self.file_buffer.Wg.Done()
				}
			}
		}
	}
}

func NewListener(
	config_obj *config_proto.Config,
	ctx context.Context, name string,
	options api.QueueOptions) (*Listener, error) {

	subctx, cancel := context.WithCancel(ctx)

	self := &Listener{
		start:   utils.GetTime().Now(),
		id:      utils.GetId(),
		name:    name,
		output:  make(chan *ordereddict.Dict),
		ctx:     subctx,
		cancel:  cancel,
		options: options,
	}

	if options.DisableFileBuffering {
		self.disable_file_buffering = 1

	} else {
		node_name := services.GetNodeName(config_obj.Frontend)
		if services.IsMaster(config_obj) {
			node_name = "master"
		}
		if options.OwnerName != "" {
			node_name = options.OwnerName
		}

		base_name := fmt.Sprintf("journal_%s_%s_", name, node_name)
		base_name = strings.Replace(base_name, "/", "...", -1)
		file_buffer, err := NewFileBasedRingBuffer(config_obj, base_name)
		if err != nil {
			return nil, err
		}

		self.file_buffer = file_buffer
		self.file_buffer_ready = make(chan bool)

		// Pump messages from the file_buffer to our listeners.
		go self.pumpFileBufferToOutput(subctx)
	}
	return self, nil
}
