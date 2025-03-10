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
	"www.velocidex.com/golang/velociraptor/file_store/tests"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	mu sync.Mutex
)

// A queue pool is an in-process listener for events.
type Listener struct {
	id      int64
	Channel chan *ordereddict.Dict
	name    string
}

type QueuePool struct {
	mu sync.Mutex
	id uint64

	config_obj *config_proto.Config

	registrations map[string][]*Listener
}

func (self *QueuePool) GetWatchers() []string {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := make([]string, 0, len(self.registrations))
	for name := range self.registrations {
		result = append(result, name)
	}

	return result
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

func (self *QueuePool) BroadcastJsonl(vfs_path string, jsonl []byte) {
	// Ensure we do not hold the lock for very long here.
	registrations := self.getRegistrations(vfs_path)
	if len(registrations) > 0 {
		// If there are any registrations, we must parse the JSON and
		// relay each row to each listener - this is expensive but
		// necessary.
		rows, err := utils.ParseJsonToDicts(jsonl)
		if err == nil {
			for _, row := range rows {
				for _, item := range registrations {
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
		}
	}
}

func (self *QueuePool) Broadcast(vfs_path string, row *ordereddict.Dict) {
	registrations := self.getRegistrations(vfs_path)
	// Ensure we do not hold the lock for very long here.
	for _, item := range registrations {
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
		id:            utils.GetId(),
		config_obj:    config_obj,
		registrations: make(map[string][]*Listener),
	}
}

type MemoryQueueManager struct {
	FileStore  api.FileStore
	pool       *QueuePool
	config_obj *config_proto.Config
}

func (self *MemoryQueueManager) Debug() {
	switch t := self.FileStore.(type) {
	case tests.Debugger:
		t.Debug()
	}
}

func (self *MemoryQueueManager) Broadcast(
	path_manager api.PathManager, dict_rows []*ordereddict.Dict) {
	for _, row := range dict_rows {
		// Set a timestamp per event for easier querying.
		row.Set("_ts", int(utils.GetTime().Now().Unix()))
		self.pool.Broadcast(path_manager.GetQueueName(), row)
	}
}

func (self *MemoryQueueManager) PushEventJsonl(
	path_manager api.PathManager, jsonl []byte, row_count int) error {

	// Writes are asyncronous
	rs_writer, err := result_sets.NewTimedResultSetWriter(
		self.config_obj, path_manager, json.DefaultEncOpts(),
		utils.BackgroundWriter)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	jsonl = json.AppendJsonlItem(jsonl, "_ts",
		int(utils.GetTime().Now().Unix()))
	rs_writer.WriteJSONL(jsonl, row_count)

	self.pool.BroadcastJsonl(path_manager.GetQueueName(), jsonl)
	return nil
}

func (self *MemoryQueueManager) PushEventRows(
	path_manager api.PathManager, dict_rows []*ordereddict.Dict) error {

	// Writes are asyncronous
	rs_writer, err := result_sets.NewTimedResultSetWriter(
		self.config_obj, path_manager, json.DefaultEncOpts(),
		utils.BackgroundWriter)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	for _, row := range dict_rows {
		// Set a timestamp per event for easier querying.
		row.Set("_ts", int(utils.GetTime().Now().Unix()))
		rs_writer.Write(row)
		self.pool.Broadcast(path_manager.GetQueueName(), row)
	}
	return nil
}

func (self *MemoryQueueManager) GetWatchers() []string {
	return self.pool.GetWatchers()
}

func (self *MemoryQueueManager) Watch(
	ctx context.Context, queue_name string,
	queue_options *api.QueueOptions) (output <-chan *ordereddict.Dict, cancel func()) {
	return self.pool.Register(queue_name)
}

func NewMemoryQueueManager(config_obj *config_proto.Config,
	file_store api.FileStore) api.QueueManager {
	return &MemoryQueueManager{
		FileStore:  file_store,
		pool:       NewQueuePool(config_obj),
		config_obj: config_obj,
	}
}
