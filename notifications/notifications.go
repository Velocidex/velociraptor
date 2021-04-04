package notifications

import (
	"context"
	"regexp"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golang.org/x/time/rate"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

var (
	notificationCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "frontend_notification_count",
		Help: "Number of notifications we issue.",
	})
)

type NotificationPool struct {
	mu      sync.Mutex
	clients map[string]chan bool
	done    chan bool
}

func NewNotificationPool() *NotificationPool {
	return &NotificationPool{
		clients: make(map[string]chan bool),
		done:    make(chan bool),
	}
}

func (self *NotificationPool) IsClientConnected(client_id string) bool {
	self.mu.Lock()
	_, pres := self.clients[client_id]
	self.mu.Unlock()

	return pres
}

func (self *NotificationPool) Listen(client_id string) (chan bool, func()) {
	new_c := make(chan bool)

	self.mu.Lock()

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
		defer close(c)
		delete(self.clients, client_id)
	}
	self.clients[client_id] = new_c
	self.mu.Unlock()

	return new_c, func() {
		self.mu.Lock()
		c, pres := self.clients[client_id]
		if pres {
			defer close(c)
			delete(self.clients, client_id)
		}
		self.mu.Unlock()
	}
}

func (self *NotificationPool) Notify(client_id string) {
	self.mu.Lock()
	c, pres := self.clients[client_id]
	if pres {
		notificationCounter.Inc()
		defer close(c)
		delete(self.clients, client_id)
	}
	self.mu.Unlock()
}

func (self *NotificationPool) NotifyByRegex(
	config_obj *config_proto.Config, re *regexp.Regexp) {

	// First take a snapshot of the current clients connected.
	self.mu.Lock()
	snapshot := make([]string, 0, len(self.clients))
	for key := range self.clients {
		if re.MatchString(key) {
			snapshot = append(snapshot, key)
		}
	}
	self.mu.Unlock()

	// Now notify all these clients in the background if
	// possible. Take it slow so as not to overwhelm the server.
	limiter_rate := rate.Limit(config_obj.Frontend.Resources.NotificationsPerSecond)
	subctx, cancel := context.WithCancel(context.Background())
	limiter := rate.NewLimiter(limiter_rate, 1)

	go func() {
		select {
		case <-self.done:
			cancel()
		}
	}()

	go func() {
		for _, client_id := range snapshot {
			self.mu.Lock()
			c, pres := self.clients[client_id]
			if pres {
				notificationCounter.Inc()
				close(c)
				delete(self.clients, client_id)
			}
			self.mu.Unlock()
			limiter.Wait(subctx)
		}
	}()
}

func (self *NotificationPool) Shutdown() {
	self.mu.Lock()
	defer self.mu.Unlock()

	close(self.done)

	// Send all the readers the quit signal and shut down the
	// pool.
	for _, c := range self.clients {
		close(c)
	}

	self.clients = make(map[string]chan bool)
}

func (self *NotificationPool) NotifyAll(config_obj *config_proto.Config) {
	self.NotifyByRegex(config_obj, regexp.MustCompile("."))
}
