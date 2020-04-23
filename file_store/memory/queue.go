// A memory based queue manager.

// This is suitable to run in-process for a single frontend. This
// queue manager allows listeners to register their interest in a
// particular event queue. When events are sent to this queue, the
// events will be broadcast to all listeners.

package memory

import (
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

var (
	pool = NewQueuePool()
)

// A queue pool is an in-process listener for events.
type Listener struct {
	id      int64
	Channel chan *ordereddict.Dict
}

type QueuePool struct {
	mu sync.Mutex

	registrations map[string][]*Listener
}

func (self *QueuePool) Register(vfs_path string) (<-chan *ordereddict.Dict, func()) {
	self.mu.Lock()
	defer self.mu.Unlock()

	registrations, _ := self.registrations[vfs_path]
	new_registration := &Listener{
		Channel: make(chan *ordereddict.Dict, 1000),
		id:      time.Now().UnixNano(),
	}
	registrations = append(registrations, new_registration)

	self.registrations[vfs_path] = registrations

	return new_registration.Channel, func() {
		self.unregister(vfs_path, new_registration.id)
	}
}

func (self *QueuePool) unregister(vfs_path string, id int64) {
	self.mu.Lock()
	defer self.mu.Unlock()

	registrations, pres := self.registrations[vfs_path]
	if pres {
		new_registrations := make([]*Listener, 0, len(registrations))
		for _, item := range registrations {
			if id == item.id {
				close(item.Channel)
			} else {
				new_registrations = append(new_registrations,
					item)
			}
		}

		self.registrations[vfs_path] = new_registrations
	}
}

func (self *QueuePool) Broadcast(vfs_path string, row *ordereddict.Dict) {
	self.mu.Lock()
	defer self.mu.Unlock()

	registrations, ok := self.registrations[vfs_path]
	if ok {
		for _, item := range registrations {
			item.Channel <- row
		}
	}
}

func NewQueuePool() *QueuePool {
	return &QueuePool{
		registrations: make(map[string][]*Listener),
	}
}

type MemoryQueueManager struct {
	file_store api.FileStore
}

func (self *MemoryQueueManager) PushRow(
	queue_name, source string, mode int, row *ordereddict.Dict) error {
	pool.Broadcast(queue_name,
		row.Set("ClientId", source).
			Set("_ts", int(time.Now().Unix())))

	return nil
}

func (self *MemoryQueueManager) Push(
	queue_name, source string, mode int, serialized_rows []byte) error {
	rows, err := utils.ParseJsonToDicts(serialized_rows)
	if err != nil {
		return err
	}

	for _, row := range rows {
		pool.Broadcast(queue_name,
			row.Set("ClientId", source).
				Set("_ts", int(time.Now().Unix())))
	}

	return nil
}

func (self *MemoryQueueManager) Read(
	queue_name, source string, start_time, endtime time.Time) <-chan vfilter.Row {
	output := make(chan vfilter.Row)

	go func() {
		defer close(output)
	}()

	return output
}

func (self *MemoryQueueManager) Watch(
	queue_name string) (output <-chan *ordereddict.Dict, cancel func()) {
	return pool.Register(queue_name)
}

func NewMemoryQueueManager(config_obj *config_proto.Config,
	file_store api.FileStore) api.QueueManager {
	return &MemoryQueueManager{
		file_store: file_store,
	}
}
