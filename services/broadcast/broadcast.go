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
	"www.velocidex.com/golang/velociraptor/utils"
)

type BroadcastService struct {
	pool *directory.QueuePool

	mu sync.Mutex

	generators map[string]<-chan *ordereddict.Dict

	// A list of watchers listening on a topic.
	// When the topic broadcaster cancels, we close all watchers.
	// When each watcher ends we remove it from the broadcast queue.
	// The following is a map of topics, and unique IDs.
	listener_closers map[string]map[uint64]func()
}

func (self *BroadcastService) RegisterGenerator(
	input <-chan *ordereddict.Dict, name string) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	_, pres := self.generators[name]
	if pres {
		return services.AlreadyRegisteredError
	}

	self.generators[name] = input

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

	delete(self.generators, name)
	closers, ok := self.listener_closers[name]

	if ok {
		for _, closer := range closers {
			closer()
		}
	}

	delete(self.listener_closers, name)
}

func (self *BroadcastService) WaitForListeners(
	ctx context.Context, name string, count int64) {
	for {
		select {
		case <-ctx.Done():
			return

		case <-time.After(100 * time.Millisecond):
			self.mu.Lock()
			closers, _ := self.listener_closers[name]
			listeners := int64(len(closers))
			self.mu.Unlock()

			if listeners >= count {
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

	_, pres := self.generators[name]
	if !pres {
		return nil, nil, fmt.Errorf("No generator registered for %v", name)
	}

	output_chan, cancel := self.pool.Register(ctx, name, options)
	// If closers is nil we create a new slice.
	closers, pres := self.listener_closers[name]
	if !pres {
		closers = make(map[uint64]func())
	}

	id := utils.GetId()
	closers[id] = cancel
	self.listener_closers[name] = closers

	return output_chan, func() {
		self.mu.Lock()
		closers, pres := self.listener_closers[name]
		if !pres {
			self.mu.Unlock()
			return
		}

		cancel, pres := closers[id]
		if !pres {
			self.mu.Unlock()
			return
		}

		delete(closers, id)
		self.mu.Unlock()

		cancel()
	}, nil
}

func NewBroadcastService(
	config_obj *config_proto.Config) services.BroadcastService {
	return &BroadcastService{
		pool:             directory.NewQueuePool(config_obj),
		generators:       make(map[string]<-chan *ordereddict.Dict),
		listener_closers: make(map[string]map[uint64]func()),
	}
}
