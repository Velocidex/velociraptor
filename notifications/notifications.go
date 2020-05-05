package notifications

import (
	"errors"
	"sync"
)

type NotificationPool struct {
	mu      sync.Mutex
	clients map[string]chan bool
}

func NewNotificationPool() *NotificationPool {
	return &NotificationPool{
		clients: make(map[string]chan bool),
	}
}

func (self *NotificationPool) IsClientConnected(client_id string) bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	_, pres := self.clients[client_id]
	return pres
}

func (self *NotificationPool) Listen(client_id string) (chan bool, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Close any old channels and make a new one.
	c, pres := self.clients[client_id]
	if pres {
		// This could happen because the client was connected,
		// but the connection is dropped and the HTTP receiver
		// is still blocked. This unblocks the old connection
		// and returns an error on the new connection at the
		// same time. If the old client is still connected, it
		// will reconnect immediately but the new client will
		// wait the full max poll to retry.
		close(c)
		delete(self.clients, client_id)

		return nil, errors.New("Only one listener may exist.")
	}

	c = make(chan bool)
	self.clients[client_id] = c

	return c, nil
}

func (self *NotificationPool) Notify(client_id string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	c, pres := self.clients[client_id]
	if pres {
		close(c)
		delete(self.clients, client_id)
	}
}

func (self *NotificationPool) Shutdown() {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Send all the readers the quit signal and shut down the
	// pool.
	for _, c := range self.clients {
		close(c)
	}

	self.clients = make(map[string]chan bool)
}

func (self *NotificationPool) NotifyAll() {
	self.mu.Lock()
	defer self.mu.Unlock()

	for _, c := range self.clients {
		close(c)
	}

	self.clients = make(map[string]chan bool)
}
