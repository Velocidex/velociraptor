package broadcast

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/directory"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/velociraptor/utils"
)

type listener struct {
	started time.Time
	id      uint64
	closer  func()
}

type watcher struct {
	started   time.Time
	name      string
	listeners map[uint64]*listener
}

type BroadcastService struct {
	pool *directory.QueuePool

	mu sync.Mutex

	// A list of watchers listening on a topic.
	// When the topic broadcaster cancels, we close all watchers.
	// When each watcher ends we remove it from the broadcast queue.
	// The following is a map of topics, and unique IDs.
	generators map[string]*watcher
}

func (self *BroadcastService) RegisterGenerator(
	input <-chan *ordereddict.Dict, name string) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	_, pres := self.generators[name]
	if pres {
		return services.AlreadyRegisteredError
	}

	new_watcher := &watcher{
		started:   utils.GetTime().Now(),
		name:      name,
		listeners: make(map[uint64]*listener),
	}

	self.generators[name] = new_watcher

	go func() {
		defer self.unregister(name)

		// Read items from the input channel and broadcast them to all
		// listeners.
		for item := range input {
			self.pool.Broadcast(name, item)
		}
	}()

	return nil
}

// Remove the generator and close off all listeners.
func (self *BroadcastService) unregister(name string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	watcher, pres := self.generators[name]
	if !pres {
		return
	}

	for _, listener := range watcher.listeners {
		listener.closer()
	}
	delete(self.generators, name)
}

func (self *BroadcastService) WaitForListeners(
	ctx context.Context, name string, count int64) {
	for {
		select {
		case <-ctx.Done():
			return

		case <-time.After(utils.Jitter(100 * time.Millisecond)):
			self.mu.Lock()
			watcher, pres := self.generators[name]
			if !pres {
				continue
			}

			listener_count := len(watcher.listeners)
			self.mu.Unlock()

			if int64(listener_count) >= count {
				return
			}
		}
	}
}

func (self *BroadcastService) Watch(
	ctx context.Context, name string, options api.QueueOptions) (
	output <-chan *ordereddict.Dict, cancel func(), err error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	watcher, pres := self.generators[name]
	if !pres {
		return nil, nil, fmt.Errorf("No generator registered for %v", name)
	}

	output_chan, cancel := self.pool.Register(ctx, name, options)

	new_listener := &listener{
		started: utils.GetTime().Now(),
		id:      utils.GetId(),
		closer:  cancel,
	}

	watcher.listeners[new_listener.id] = new_listener

	return output_chan, func() {
		self.mu.Lock()

		id := new_listener.id
		listener, pres := watcher.listeners[id]
		if !pres {
			self.mu.Unlock()
			return
		}
		listener.closer()

		delete(watcher.listeners, id)

		if len(watcher.listeners) == 0 {
			delete(self.generators, name)
		}

		self.mu.Unlock()

	}, nil
}

func NewBroadcastService(
	config_obj *config_proto.Config) services.BroadcastService {
	res := &BroadcastService{
		pool:       directory.NewQueuePool(config_obj),
		generators: make(map[string]*watcher),
	}

	debug.RegisterProfileWriter(debug.ProfileWriterInfo{
		Name:          "BroadcastService-" + utils.GetOrgId(config_obj),
		Description:   "Track generators installed via the generator() plugin.",
		ProfileWriter: res.ProfileWriter,
		Categories:    []string{"Org", services.GetOrgName(config_obj), "Services"},
	})

	return res
}
