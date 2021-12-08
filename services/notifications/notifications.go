package notifications

// Notifications are low latency indications that something has
// changed. Callers may listen for notifications using the
// ListenForNotification() function which returns a channel. The
// caller can then block on the channel until an event occurs, at
// which time, the channel will be closed.

// Notifications do not carry actual data, they just indicate that an
// event occured. Callers need to go back to actually do something
// with that information (read the filestore etc). Notifications are
// not meant to be reliable - it is possible to miss a notification or
// to receive too many notifications while no change
// occurs. Notifications are just an optimization that reduces the
// need to poll something.

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/notifications"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/journal"
)

var (
	timeoutClientPing = promauto.NewCounter(prometheus.CounterOpts{
		Name: "client_ping_timeout",
		Help: "Number of times the client ping has timed out.",
	})

	notificationsSentCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "notifications_send_count",
		Help: "Number of notification messages sent.",
	})

	notificationsReceivedCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "notifications_receive_count",
		Help: "Number of notification messages received.",
	})

	isClientConnectedHistorgram = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "is_client_connected_latency",
			Help:    "How long it takes to establish if a client is connected.",
			Buckets: prometheus.LinearBuckets(0.1, 1, 10),
		},
	)
)

type Notifier struct {
	pool_mu           sync.Mutex
	notification_pool *notifications.NotificationPool

	idx uint64
}

// The notifier service watches for events from
// Server.Internal.Notifications and notifies the notification pool in
// the current process. This allows multiprocess communication as the
// notifications may arrive from other frontend processes through the
// journal service.
func StartNotificationService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	self := &Notifier{
		notification_pool: notifications.NewNotificationPool(),
	}
	services.RegisterNotifier(self)

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> the notification service.")

	err := journal.WatchQueueWithCB(ctx, config_obj, wg,
		"Server.Internal.Ping", self.ProcessPing)
	if err != nil {
		return err
	}

	// Watch the journal.
	journal_service, err := services.GetJournal()
	if err != nil {
		return err
	}
	events, cancel := journal_service.Watch(ctx, "Server.Internal.Notifications")

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()

		defer services.RegisterNotifier(nil)
		defer func() {
			self.pool_mu.Lock()
			defer self.pool_mu.Unlock()

			self.notification_pool.Shutdown()
			self.notification_pool = nil
		}()
		defer logger.Info("Exiting notification service!")

		for {
			select {
			case <-ctx.Done():
				return

			case event, ok := <-events:
				if !ok {
					return
				}

				target, ok := event.GetString("Target")
				if !ok {
					continue
				}
				notificationsReceivedCounter.Inc()
				self.notification_pool.Notify(target)
			}
		}
	}()

	return nil
}

// When receiving a Ping request, we simply notify the target if the
// ClientId is currently connected to this server.
func (self *Notifier) ProcessPing(ctx context.Context,
	config_obj *config_proto.Config,
	row *ordereddict.Dict) error {
	client_id, pres := row.GetString("ClientId")
	if !pres {
		return nil
	}

	if !self.notification_pool.IsClientConnected(client_id) {
		return nil
	}

	/*
		// Client is directly connected - inform the client info
		// manager. Normally the Ping is sent by the frontend to find out
		// which clients are connected to a minion. In this case it is
		// worth the extra ping updates to deliver fresh data to the GUI -
		// there are not too many clients but we need to know accurate
		// data.
		client_info_manager, err := services.GetClientInfoManager()
		if err != nil {
			return err
		}

		client_info_manager.UpdateStats(client_id, func(stats *services.Stats) {
			stats.Ping =
		})
	*/
	notify_target, pres := row.GetString("NotifyTarget")
	if !pres {
		return nil
	}

	// Notify the target of the Ping.
	return self.NotifyListener(config_obj, notify_target, "ClientPing")
}

func (self *Notifier) ListenForNotification(client_id string) (chan bool, func()) {
	self.pool_mu.Lock()
	if self.notification_pool == nil {
		self.notification_pool = notifications.NewNotificationPool()
	}
	notification_pool := self.notification_pool
	self.pool_mu.Unlock()

	return notification_pool.Listen(client_id)
}

func (self *Notifier) NotifyListener(config_obj *config_proto.Config,
	id, tag string) error {
	journal, err := services.GetJournal()
	if err != nil {
		return err
	}

	// We need to send this ASAP so we do not use an async send.
	notificationsSentCounter.Inc()
	return journal.PushRowsToArtifact(config_obj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("Tag", tag).
			Set("Target", id)},
		"Server.Internal.Notifications", "server", "",
	)
}

func (self *Notifier) NotifyDirectListener(client_id string) {
	if self.notification_pool.IsClientConnected(client_id) {
		self.notification_pool.Notify(client_id)
	}
}

func (self *Notifier) NotifyListenerAsync(config_obj *config_proto.Config,
	id, tag string) {
	journal, err := services.GetJournal()
	if err != nil {
		return
	}

	notificationsSentCounter.Inc()
	journal.PushRowsToArtifactAsync(config_obj,
		ordereddict.NewDict().
			Set("Tag", tag).
			Set("Target", id),
		"Server.Internal.Notifications")
}

func (self *Notifier) IsClientDirectlyConnected(client_id string) bool {
	return self.notification_pool.IsClientConnected(client_id)
}

func (self *Notifier) ListClients() []string {
	return self.notification_pool.ListClients()
}

func (self *Notifier) IsClientConnected(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string, timeout int) bool {

	timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
		isClientConnectedHistorgram.Observe(v)
	}))
	defer timer.ObserveDuration()

	// Shotcut if the client is directly connected.
	if self.IsClientDirectlyConnected(client_id) {
		return true
	}

	// Get a unique ID
	idx := atomic.AddUint64(&self.idx, 1)

	// Watch for Ping replies on this notification.
	id := fmt.Sprintf("IsClientConnected%v", idx)
	done, cancel := self.ListenForNotification(id)
	defer cancel()

	// Send ping to all nodes, they will reply with a
	// notification.
	journal, err := services.GetJournal()
	if err != nil {
		return false
	}

	err = journal.PushRowsToArtifact(config_obj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("ClientId", client_id).
			Set("NotifyTarget", id)},
		"Server.Internal.Ping", "server", "")
	if err != nil {
		return false
	}

	if timeout == 0 {
		return false
	}

	// Now wait here for the reply.
	select {
	case <-done:
		// Client is found!
		return true

	case <-time.After(time.Duration(timeout) * time.Second):
		timeoutClientPing.Inc()
		// Nope - not found within the timeout.
		return false
	}
}
