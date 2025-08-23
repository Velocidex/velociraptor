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
	"sort"
	"sync"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

// A Queue manages a set of registrations at a specific queue name
// (artifact name).
type QueuePool struct {
	mu sync.Mutex

	config_obj *config_proto.Config

	registrations map[string][]*Listener
}

func (self *QueuePool) Stats() *ordereddict.Dict {
	self.mu.Lock()
	defer self.mu.Unlock()

	res := ordereddict.NewDict()
	for k, listeners := range self.registrations {
		var stats []*ordereddict.Dict
		for _, l := range listeners {
			stats = append(stats, l.Stats())
		}
		res.Set(k, stats)
	}
	return res
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

func (self *QueuePool) Register(
	ctx context.Context, vfs_path string,
	options api.QueueOptions) (<-chan *ordereddict.Dict, func()) {

	self.mu.Lock()
	defer self.mu.Unlock()

	registrations := self.registrations[vfs_path]

	subctx, cancel := context.WithCancel(ctx)
	new_registration, err := NewListener(self.config_obj, subctx, vfs_path, options)
	if err != nil {
		logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
		logger.Warn("Failed to register QueuePool for %s: %v", vfs_path, err)
		cancel()
		output_chan := make(chan *ordereddict.Dict)
		close(output_chan)
		return output_chan, cancel
	}

	registrations = append(registrations, new_registration)

	self.registrations[vfs_path] = registrations

	return new_registration.Output(), func() {
		self.unregister(vfs_path, new_registration.id)
		cancel()
	}
}

// This holds a lock on the entire pool and it is used when the system
// shuts down so not very often.
func (self *QueuePool) unregister(vfs_path string, id uint64) (found bool) {
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
		if len(new_registrations) == 0 {
			delete(self.registrations, vfs_path)
		}
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
					item.Send(row)
				}
			}
		}
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
}

func (self *DirectoryQueueManager) Debug() *ordereddict.Dict {
	return self.queue_pool.Debug()
}

// Sends the events without writing them to the filestore.
func (self *DirectoryQueueManager) Broadcast(
	path_manager api.PathManager, dict_rows []*ordereddict.Dict) {
	for _, row := range dict_rows {
		// Set a timestamp per event for easier querying.
		row.Set("_ts", int(utils.GetTime().Now().Unix()))
		self.queue_pool.Broadcast(path_manager.GetQueueName(), row)
	}
}

func (self *DirectoryQueueManager) PushEventRows(
	path_manager api.PathManager, dict_rows []*ordereddict.Dict) error {

	// Writes are asyncronous.
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
		self.queue_pool.Broadcast(path_manager.GetQueueName(), row)
	}
	return nil
}

func (self *DirectoryQueueManager) PushEventJsonl(
	path_manager api.PathManager, jsonl []byte, row_count int) error {

	// Writes are asyncronous.
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
	self.queue_pool.BroadcastJsonl(path_manager.GetQueueName(), jsonl)

	return nil
}

func (self *DirectoryQueueManager) GetWatchers() []string {
	return self.queue_pool.GetWatchers()
}

func (self *DirectoryQueueManager) Watch(
	ctx context.Context, queue_name string,
	queue_options *api.QueueOptions) (<-chan *ordereddict.Dict, func()) {

	if queue_options == nil {
		queue_options = &api.QueueOptions{}
	}

	// If the caller of Watch no longer cares about watching the queue
	// they will call the cancellation function. This must abandon the
	// current queue listener and cause any outstanding events to be
	// dropped on the floor.
	subctx, cancel := context.WithCancel(ctx)
	output_chan, pool_cancel := self.queue_pool.Register(
		subctx, queue_name, *queue_options)
	return output_chan, func() {
		cancel()
		pool_cancel()
	}
}

func NewDirectoryQueueManager(config_obj *config_proto.Config,
	file_store api.FileStore) api.QueueManager {

	result := &DirectoryQueueManager{
		FileStore:  file_store,
		config_obj: config_obj,
		queue_pool: NewQueuePool(config_obj),
	}

	debug.RegisterProfileWriter(debug.ProfileWriterInfo{
		Name:        "QueueManager " + services.GetOrgName(config_obj),
		Categories:  []string{"Org", services.GetOrgName(config_obj), "Services"},
		Description: "Report the current states of server artifact event queues.",
		ProfileWriter: func(ctx context.Context,
			scope vfilter.Scope, output_chan chan vfilter.Row) {

			d := result.Debug()
			items := d.Items()
			sort.Slice(items, func(i, j int) bool {
				return items[i].Key < items[j].Key
			})

			for _, i := range items {
				output_chan <- ordereddict.NewDict().
					Set("Type", "QueueManager").
					Set("Org", services.GetOrgName(config_obj)).
					Set("Name", i.Key).
					Set("Listener", i.Value)
			}
		},
	})

	return result
}
