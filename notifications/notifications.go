package notifications

import (
	"fmt"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	notificationCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "frontend_notification_count",
		Help: "Number of notifications we issue.",
	})
)

type clientNotifier struct {
	id   uint64
	done chan bool
}

type NotificationPool struct {
	mu      sync.Mutex
	clients map[string]clientNotifier
	done    chan bool
}

func NewNotificationPool() *NotificationPool {
	return &NotificationPool{
		clients: make(map[string]clientNotifier),
		done:    make(chan bool),
	}
}

func (self *NotificationPool) Count() uint64 {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := uint64(0)
	for k := range self.clients {
		// Only report real clients waiting for notifications.
		if strings.HasPrefix(k, "C.") {
			result++
		}
	}

	return result
}

func (self *NotificationPool) ListClients() []string {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := make([]string, 0, len(self.clients))
	for k := range self.clients {
		result = append(result, k)
	}
	return result
}

func (self *NotificationPool) IsClientConnected(client_id string) bool {
	self.mu.Lock()
	_, pres := self.clients[client_id]
	self.mu.Unlock()

	return pres
}

func (self *NotificationPool) DebugPrint() {
	self.mu.Lock()
	fmt.Printf("Clients connected: ")
	for k := range self.clients {
		fmt.Printf("%v ", k)
	}
	fmt.Printf("\n")
	self.mu.Unlock()
}

func (self *NotificationPool) Listen(client_id string) (chan bool, func()) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Close any old channels and make a new one.
	client_notifier, pres := self.clients[client_id]
	if pres {
		// This could happen because the client was connected, but the
		// connection is dropped and the HTTP receiver is still
		// blocked. This unblocks the old connection and returns an
		// error on the new connection at the same time. If the old
		// client is still connected, it will reconnect immediately
		// but the new client will wait the full max poll to retry.
		defer close(client_notifier.done)
		delete(self.clients, client_id)
	}

	new_c := make(chan bool)
	new_id := utils.GetId()
	self.clients[client_id] = clientNotifier{
		id:   new_id,
		done: new_c,
	}

	return new_c, func() {
		self.mu.Lock()
		defer self.mu.Unlock()

		client_notifier, pres := self.clients[client_id]

		// Only close our own notification. If a second listener
		// appears for the same tag after we installed our own tag, we
		// dont close it!
		if pres && client_notifier.id == new_id {
			defer close(client_notifier.done)
			delete(self.clients, client_id)
		}
	}
}

func (self *NotificationPool) Notify(client_id string) {
	self.mu.Lock()
	client_notifier, pres := self.clients[client_id]
	if pres {
		notificationCounter.Inc()
		defer close(client_notifier.done)
		delete(self.clients, client_id)
	}
	self.mu.Unlock()
}

func (self *NotificationPool) Shutdown() {
	self.mu.Lock()
	defer self.mu.Unlock()

	close(self.done)

	// Send all the readers the quit signal and shut down the
	// pool.
	for _, client_notifier := range self.clients {
		close(client_notifier.done)
	}

	self.clients = make(map[string]clientNotifier)
}
