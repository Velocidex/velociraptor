// A Queue manager that uses files on disk.

// The queue manager is a broker between writers and readers. Writers
// want to emit a message to a queue with minimumal delay, and have
// the message dispatched to all readers with minimal latency.

// A memory queue simply pushes the message to all reader's via a
// buffered channel. As long as the channel buffer remains available
// this works well with very minimal latency in broadcasting to
// readers. However, when the channel becomes full the writers may be
// blocked while readers are working their way through the channel.

// This queue manager uses a combination of a channel and a disk file
// to buffer messages for readers. When a writer writes to the queue
// manager, the manager attempts to write on the channel but if it is
// not available, then the writer switches to a ring buffer file on
// disk.  A separate go routine drains the disk file into the channel
// periodically. Therefore, we never block the writer - either the
// message is delivered immediately to the buffered channel, or it is
// written to disk and later delivered.

// This low latency property is critical because queue managers are
// used to deliver messages in critical code paths and can not be
// delayed.

package directory

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
)

// A listener wraps a channel that our client will listen on. We send
// the message to each listener that is subscribed to the queue.
type Listener struct {
	id int64

	// The consumer interested in these events. The consumer may
	// block arbitrarily.
	output chan *ordereddict.Dict

	// We receive events on this channel - we guarantee this does
	// not block for long.
	input chan *ordereddict.Dict

	// A backup file to store extra messages.
	file_buffer *FileBasedRingBuffer

	// Name of the file_buffer
	tmpfile string

	// The context of the creator of this listener - When it is
	// done we drop messages to it.
	ctx context.Context
}

// Should not block - very fast.
func (self *Listener) Send(item *ordereddict.Dict) {
	select {
	case <-self.ctx.Done():
		return

	case self.input <- item:
	}
}

// Flush the file buffer into the output channel.
func (self *Listener) FlushFile() {
	// Immediately drain the file.
	for {
		items := self.file_buffer.Lease(100)
		if len(items) == 0 {
			return
		}
		for _, item := range items {
			select {
			case <-self.ctx.Done():
				// We still need to release this item from the wg.
				self.file_buffer.Wg.Done()

			case self.output <- item:
				self.file_buffer.Wg.Done()
			}
		}
	}
}

func (self *Listener) Close() {
	self.FlushFile()

	// Wait for all outstanding file buffer messages to be sent.
	self.file_buffer.Wg.Wait()
	self.file_buffer.Close()

	os.Remove(self.tmpfile) // clean up file buffer
}

func (self *Listener) Debug() *ordereddict.Dict {
	result := ordereddict.NewDict().Set("BackingFile", self.tmpfile)
	st, _ := os.Stat(self.tmpfile)
	result.Set("Size", int64(st.Size()))

	return result
}

func NewListener(config_obj *config_proto.Config, ctx context.Context,
	output chan *ordereddict.Dict) (*Listener, error) {

	tmpfile, err := ioutil.TempFile("", "journal")
	if err != nil {
		return nil, err
	}

	file_buffer, err := NewFileBasedRingBuffer(config_obj, tmpfile)
	if err != nil {
		return nil, err
	}

	self := &Listener{
		id:          time.Now().UnixNano(),
		ctx:         ctx,
		input:       make(chan *ordereddict.Dict),
		output:      output,
		file_buffer: file_buffer,
		tmpfile:     tmpfile.Name(),
	}

	// Pump messages from input channel and distribute to
	// output. If output is busy we divert to the file buffer.
	go func() {
		defer self.Close()

		for {
			select {
			case <-ctx.Done():
				return

			case item, ok := <-self.input:
				if !ok {
					return
				}
				select {
				case <-ctx.Done():
					return

					// If we can immediately push
					// to the output, do so
				case output <- item:

					// Otherwise push to the file.
				default:
					self.file_buffer.Enqueue(item)
				}
			}
		}

	}()

	// Pump messages from the file_buffer to our listeners.
	go func() {
		for {
			// Wait here until the file has some data in it.
			select {
			case <-ctx.Done():
				return

			case <-time.After(time.Second):
				// Get some messages from the file.
				items := self.file_buffer.Lease(100)
				for _, item := range items {
					select {
					case <-ctx.Done():
						self.file_buffer.Wg.Done()

					case output <- item:
						self.file_buffer.Wg.Done()
					}
				}
			}
		}
	}()

	return self, nil
}

// A Queue manages a set of registrations at a specific queue name
// (artifact name).
type QueuePool struct {
	mu sync.Mutex

	config_obj *config_proto.Config

	registrations map[string][]*Listener
}

func (self *QueuePool) Register(
	ctx context.Context, vfs_path string) (<-chan *ordereddict.Dict, func()) {
	self.mu.Lock()
	defer self.mu.Unlock()

	output_chan := make(chan *ordereddict.Dict)

	registrations := self.registrations[vfs_path]

	subctx, cancel := context.WithCancel(ctx)
	new_registration, err := NewListener(self.config_obj, subctx, output_chan)
	if err != nil {
		close(output_chan)
		cancel()

		return output_chan, func() {}
	}

	registrations = append(registrations, new_registration)

	self.registrations[vfs_path] = registrations

	return output_chan, func() {
		found := self.unregister(vfs_path, new_registration.id)
		if found {
			cancel()
			close(output_chan)
		}
	}
}

// This holds a lock on the entire pool and it is used when the system
// shuts down so not very often.
func (self *QueuePool) unregister(vfs_path string, id int64) (found bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	registrations, pres := self.registrations[vfs_path]
	if pres {
		new_registrations := make([]*Listener, 0, len(registrations))
		for _, item := range registrations {
			if id == item.id {
				item.Close()
				found = true

			} else {
				new_registrations = append(new_registrations, item)
			}
		}

		self.registrations[vfs_path] = new_registrations
	}

	return found
}

// Make a copy of the registrations under lock and then we can take
// our time to send them later.
func (self *QueuePool) getRegistrations(vfs_path string) []*Listener {
	self.mu.Lock()
	defer self.mu.Unlock()

	registrations, ok := self.registrations[vfs_path]
	if ok {
		// Make a copy of the registrations for sending this
		// message.
		return append([]*Listener{}, registrations...)
	}

	return nil
}

func (self *QueuePool) Broadcast(vfs_path string, row *ordereddict.Dict) {
	// Ensure we do not hold the lock for very long here.
	for _, item := range self.getRegistrations(vfs_path) {
		item.Send(row)
	}
}

func (self *QueuePool) Debug() *ordereddict.Dict {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := ordereddict.NewDict()
	for k, v := range self.registrations {
		listeners := ordereddict.NewDict()
		for idx, l := range v {
			listeners.Set(fmt.Sprintf("%v", idx), l.Debug())
		}
		result.Set(k, listeners)
	}
	return result
}

func NewQueuePool(config_obj *config_proto.Config) *QueuePool {
	return &QueuePool{
		config_obj:    config_obj,
		registrations: make(map[string][]*Listener),
	}
}

type DirectoryQueueManager struct {
	queue_pool *QueuePool
	FileStore  api.FileStore
	config_obj *config_proto.Config
	Clock      utils.Clock
}

func (self *DirectoryQueueManager) Debug() *ordereddict.Dict {
	return self.queue_pool.Debug()
}

func (self *DirectoryQueueManager) PushEventRows(
	path_manager api.PathManager, dict_rows []*ordereddict.Dict) error {

	rs_writer, err := result_sets.NewTimedResultSetWriter(
		self.FileStore, path_manager, nil)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	for _, row := range dict_rows {
		// Set a timestamp per event for easier querying.
		row.Set("_ts", int(self.Clock.Now().Unix()))
		rs_writer.Write(row)
		self.queue_pool.Broadcast(path_manager.GetQueueName(), row)
	}
	return nil
}

func (self *DirectoryQueueManager) Watch(ctx context.Context,
	queue_name string) (<-chan *ordereddict.Dict, func()) {

	// If the caller of Watch no longer cares about watching the queue
	// they will call the cancellation function. This must abandon the
	// current queue listener and cause any outstanding events to be
	// dropped on the floor.
	subctx, cancel := context.WithCancel(ctx)
	output_chan, pool_cancel := self.queue_pool.Register(subctx, queue_name)
	return output_chan, func() {
		cancel()
		pool_cancel()
	}
}

func NewDirectoryQueueManager(config_obj *config_proto.Config,
	file_store api.FileStore) api.QueueManager {
	return &DirectoryQueueManager{
		FileStore:  file_store,
		config_obj: config_obj,
		queue_pool: NewQueuePool(config_obj),
		Clock:      utils.RealClock{},
	}
}
