package notifications

import (
	"regexp"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
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
	defer self.mu.Unlock()

	_, pres := self.clients[client_id]
	return pres
}

func (self *NotificationPool) Listen(client_id string) (chan bool, func()) {
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
	}

	c = make(chan bool)
	self.clients[client_id] = c

	return c, func() {
		self.mu.Lock()
		defer self.mu.Unlock()

		c, pres := self.clients[client_id]
		if pres {
			close(c)
			delete(self.clients, client_id)
		}
	}
}

func (self *NotificationPool) Notify(client_id string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	c, pres := self.clients[client_id]
	if pres {
		notificationCounter.Inc()
		close(c)
		delete(self.clients, client_id)
	}
}

func (self *NotificationPool) NotifyByRegex(
	config_obj *config_proto.Config, re *regexp.Regexp) {

	// First take a snapshot of the current clients connected.
	self.mu.Lock()
	snapshot := make([]string, 0, len(self.clients))
	for key, _ := range self.clients {
		if re.MatchString(key) {
			snapshot = append(snapshot, key)
		}
	}
	self.mu.Unlock()

	// Now notify all these clients in the background if
	// possible. Take it slow so as not to overwhelm the server.
	rate := config_obj.Frontend.Resources.NotificationsPerSecond
	if rate == 0 || rate > 1000 {
		rate = 1000
	}
	sleep_time := time.Duration(1000/rate) * time.Millisecond
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

			select {
			case <-self.done:
				return
			case <-time.After(sleep_time):
			}
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
