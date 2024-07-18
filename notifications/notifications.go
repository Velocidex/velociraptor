package notifications

/*
Notifications

This module implements a simple notification pool.

1. A listener calls pool.Listen(name) to listen to notifications on
   the name. The call returns a channel and a closer function.

2. A sender will notify the name by calling pool.Notify(name). If a
   listener is currently listening on that name, the channel will be
   closed.

3. Notifications are single shot - the listener will need to relisten
   to the name to receive new notifications.

This scheme ensures that senders are never blocked on notifications.

## Debouncing notifications

The intention of notifications is to announce to listeners that
something happened - the intention is not convey the exact number of
events sent. Timing is not critical or guaranteed. The mechanism is
designed for listeners which are mostly idle but need to be notified
with low latency when something is available to do.

Therefore notifications are debounced to ensure that rapid
notifications do not result in too many checking loops.

1. Race detection between listener and sender: If the listener calls
   Listen() just after a notification we ensure that the listener is
   still able to receive the notification. This ensures notifications
   are never lost.


The following cases are handled:

1. The normal case - listener called before notification is issued.
---------------------------> Time
------x        Listener
--------x      Notification

2. Listener is called a short time after Notification
---------------------------> Time
---------x        Listener
--------x         Notification

3. Debouncing: Many notifications are called quickly - listener only
   receives events slowly.

---------------------------> Time
-----------x----x----x----x        Listener
--------x-x-xx-xx-xxxx-x---        Notification


*/

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	notificationCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "frontend_notification_count",
		Help: "Number of notifications we issue.",
	})

	debounceDuration = time.Second
)

type clientNotifier struct {
	mu            sync.Mutex
	last_time     time.Time
	pre_last_time time.Time
	ctx           context.Context

	// Set to true when we delay a notification to debounce it.
	debouncing bool

	refcount int

	done chan bool
}

// Look at the last few trigger times and decide if we should fire
// again.
func (self *clientNotifier) shouldFire(now time.Time) bool {
	return now.Sub(self.last_time) > debounceDuration
}

func (self *clientNotifier) debounce(now time.Time) {
	self.debouncing = true

	go func() {
		now := utils.GetTime().Now()

		select {
		case <-self.ctx.Done():
			return

		case <-time.After(self.last_time.Add(debounceDuration).Sub(now)):
			self.mu.Lock()
			defer self.mu.Unlock()

			now := utils.GetTime().Now()

			self.debouncing = false
			close(self.done)
			self.done = nil
			self.fire(now)
			return
		}
	}()
}

func (self *clientNotifier) fire(now time.Time) {
	self.pre_last_time = self.last_time
	self.last_time = now
}

func (self *clientNotifier) Notify() {
	self.mu.Lock()
	defer self.mu.Unlock()

	now := utils.GetTime().Now()

	// We are waiting for debounce - nothing to do
	if self.debouncing {
		return
	}

	// No one is listening just record the notify time in case someone
	// starts listening soon.
	if self.done == nil {
		self.fire(now)
		return
	}

	// This notification is too soon after the last one start a
	// debounce cycle.
	if !self.shouldFire(now) {
		self.debounce(now)
		return
	}

	// We can notify right now.
	close(self.done)
	self.done = nil
	self.fire(now)
}

func (self *clientNotifier) IsActive() bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.refcount > 0
}

func (self *clientNotifier) decRef() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.refcount--
}

func (self *clientNotifier) Listen() (chan bool, func()) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.refcount++

	if self.done != nil {
		return self.done, self.decRef
	}

	self.done = make(chan bool)

	// The last notify was a short time ago, carry it over to the new
	// listener.
	now := utils.GetTime().Now()

	if now.Sub(self.last_time) < debounceDuration &&
		now.Sub(self.pre_last_time) > debounceDuration {

		// Return an already closed channel to immediately trigger the
		// notification.
		ret := self.done
		close(self.done)
		self.done = nil
		self.fire(now)
		return ret, self.decRef
	}

	return self.done, self.decRef
}

func newClientNotifier(ctx context.Context) *clientNotifier {
	return &clientNotifier{ctx: ctx}
}

type NotificationPool struct {
	mu      sync.Mutex
	clients map[string]*clientNotifier

	ctx context.Context
	wg  *sync.WaitGroup
}

func NewNotificationPool(
	ctx context.Context, wg *sync.WaitGroup) *NotificationPool {
	result := &NotificationPool{
		clients: make(map[string]*clientNotifier),
		wg:      wg,
		ctx:     ctx,
	}

	// Decrememnted in Shutdown
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()

		result.Shutdown()
	}()

	return result
}

func (self *NotificationPool) Count() uint64 {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := uint64(0)
	for k, v := range self.clients {
		// Only report real clients waiting for notifications.
		if strings.HasPrefix(k, "C.") && v.IsActive() {
			result++
		}
	}

	return result
}

func (self *NotificationPool) ListClients() []string {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := make([]string, 0, len(self.clients))
	for k, v := range self.clients {
		if v.IsActive() {
			result = append(result, k)
		}
	}
	return result
}

func (self *NotificationPool) IsClientConnected(client_id string) bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	notifier, pres := self.clients[client_id]
	if !pres || notifier == nil {
		return false
	}

	return notifier.IsActive()
}

func (self *NotificationPool) DebugPrint() {
	self.mu.Lock()
	fmt.Printf("Clients connected: ")
	for k, v := range self.clients {
		if v.IsActive() {
			fmt.Printf("%v", k)
		}
	}
	fmt.Printf("\n")
	self.mu.Unlock()
}

func (self *NotificationPool) Listen(client_id string) (chan bool, func()) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Close any old channels and make a new one.
	client_notifier, pres := self.clients[client_id]
	if !pres {
		client_notifier = newClientNotifier(self.ctx)
		self.clients[client_id] = client_notifier
	}

	return client_notifier.Listen()
}

func (self *NotificationPool) Notify(client_id string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	client_notifier, pres := self.clients[client_id]
	if !pres {
		client_notifier = newClientNotifier(self.ctx)
		self.clients[client_id] = client_notifier
	}

	notificationCounter.Inc()
	client_notifier.Notify()
}

func (self *NotificationPool) Shutdown() {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Send all the readers the quit signal and shut down the
	// pool.
	for _, client_notifier := range self.clients {
		client_notifier.Notify()
	}

	self.clients = make(map[string]*clientNotifier)
}
