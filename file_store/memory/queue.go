// A memory based queue manager.

// This is suitable to run in-process for a single frontend. This
// queue manager allows listeners to register their interest in a
// particular event queue. When events are sent to this queue, the
// events will be broadcast to all listeners.

package memory

import (
	"context"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	mu   sync.Mutex
	pool *QueuePool
)

func GlobalQueuePool(config_obj *config_proto.Config) *QueuePool {
	mu.Lock()
	defer mu.Unlock()

	if pool != nil {
		return pool
	}

	pool = NewQueuePool(config_obj)
	return pool
}

// A queue pool is an in-process listener for events.
type Listener struct {
	id      int64
	Channel chan *ordereddict.Dict
	name    string
}

type QueuePool struct {
	mu sync.Mutex

	config_obj *config_proto.Config

	registrations map[string][]*Listener
}

func (self *QueuePool) Register(vfs_path string) (<-chan *ordereddict.Dict, func()) {
	self.mu.Lock()
	defer self.mu.Unlock()

	registrations := self.registrations[vfs_path]
	new_registration := &Listener{
		Channel: make(chan *ordereddict.Dict, 1000),
		id:      time.Now().UnixNano(),
		name:    vfs_path,
	}
	registrations = append(registrations, new_registration)

	self.registrations[vfs_path] = registrations

	return new_registration.Channel, func() {
		self.unregister(vfs_path, new_registration.id)
	}
}

// This holds a lock on the entire pool and it is used when the system
// shuts down so not very often.
func (self *QueuePool) unregister(vfs_path string, id int64) {
	self.mu.Lock()
	defer self.mu.Unlock()

	registrations, pres := self.registrations[vfs_path]
	if pres {
		new_registrations := make([]*Listener, 0, len(registrations))
		for _, item := range registrations {
			if id == item.id {
				// Do not close the channel in case
				// the writer is still active. This
				// fixes a race where a channel is
				// deregistered while a broadcast is
				// still taking place.
				// close(item.Channel)
			} else {
				new_registrations = append(new_registrations,
					item)
			}
		}

		self.registrations[vfs_path] = new_registrations
	}
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
		select {
		case item.Channel <- row:
		case <-time.After(2 * time.Second):
			logger := logging.GetLogger(
				self.config_obj, &logging.FrontendComponent)
			logger.Error("QueuePool: Dropping message to queue %v",
				item.name)
		}
	}
}

func NewQueuePool(config_obj *config_proto.Config) *QueuePool {
	return &QueuePool{
		config_obj:    config_obj,
		registrations: make(map[string][]*Listener),
	}
}

type MemoryQueueManager struct {
	FileStore  api.FileStore
	config_obj *config_proto.Config
	Clock      utils.Clock
}

func (self *MemoryQueueManager) Debug() {
	switch t := self.FileStore.(type) {
	case api.Debugger:
		t.Debug()
	}
}

func (self *MemoryQueueManager) PushEventRows(
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
		GlobalQueuePool(self.config_obj).Broadcast(
			path_manager.GetQueueName(), row)
	}
	return nil
}

func (self *MemoryQueueManager) Watch(
	ctx context.Context, queue_name string) (output <-chan *ordereddict.Dict, cancel func()) {
	return GlobalQueuePool(self.config_obj).Register(queue_name)
}

func NewMemoryQueueManager(config_obj *config_proto.Config,
	file_store api.FileStore) api.QueueManager {
	return &MemoryQueueManager{
		FileStore:  file_store,
		config_obj: config_obj,
		Clock:      utils.RealClock{},
	}
}
