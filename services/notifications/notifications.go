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
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/notifications"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/utils"
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

type tracker struct {
	mu        sync.Mutex
	count     int
	connected bool
	closed    bool
	done      chan bool
}

type Notifier struct {
	mu                sync.Mutex
	notification_pool *notifications.NotificationPool

	uuid int64

	client_connection_tracker map[string]*tracker

	config_obj *config_proto.Config
}

// The notifier service watches for events from
// Server.Internal.Notifications and notifies the notification pool in
// the current process. This allows multiprocess communication as the
// notifications may arrive from other frontend processes through the
// journal service.
func NewNotificationService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.Notifier, error) {

	self := &Notifier{
		notification_pool:         notifications.NewNotificationPool(ctx, wg),
		uuid:                      utils.GetGUID(),
		client_connection_tracker: make(map[string]*tracker),
		config_obj:                config_obj,
	}

	// On clients the notifications service is local only.
	if config_obj.Services != nil &&
		config_obj.Services.ClientEventTable {
		return self, nil
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> the notification service for %v.",
		services.GetOrgName(config_obj))

	err := journal.WatchQueueWithCB(ctx, config_obj, wg,
		"Server.Internal.Ping", "NotificationService",
		self.ProcessPing)
	if err != nil {
		return nil, err
	}

	err = journal.WatchQueueWithCB(ctx, config_obj, wg,
		"Server.Internal.Pong", "NotificationService",
		self.ProcessPong)
	if err != nil {
		return nil, err
	}

	// Watch the journal.
	journal_service, err := services.GetJournal(config_obj)
	if err != nil {
		return nil, err
	}
	events, cancel := journal_service.Watch(ctx,
		"Server.Internal.Notifications", "NotificationService")

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()

		defer func() {
			self.mu.Lock()
			defer self.mu.Unlock()

			self.notification_pool.Shutdown()
		}()
		defer logger.Info("<red>Exiting</> notification service for %v!",
			services.GetOrgName(config_obj))

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

	debug.RegisterProfileWriter(debug.ProfileWriterInfo{
		Name:          "notifier-" + utils.GetOrgId(config_obj),
		Description:   "Information about directly connected clients.",
		ProfileWriter: self.WriteProfile,
		Categories:    []string{"Org", services.GetOrgName(config_obj), "Services"},
	})

	return self, nil
}

func (self *Notifier) ProcessPong(ctx context.Context,
	config_obj *config_proto.Config,
	row *ordereddict.Dict) error {

	// Ignore messages coming from us.
	from, pres := row.GetInt64("From")
	if !pres || from == 0 || from == self.uuid {
		return nil
	}

	notify_target, pres := row.GetString("NotifyTarget")
	if !pres {
		return nil
	}

	connected, pres := row.GetBool("Connected")
	if !pres {
		return nil
	}

	self.mu.Lock()
	tracker, pres := self.client_connection_tracker[notify_target]
	self.mu.Unlock()
	if !pres {
		return nil
	}

	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	if pres && !tracker.closed {
		tracker.connected = connected
		tracker.count--
		if tracker.count <= 0 && !tracker.closed {
			close(tracker.done)
			tracker.closed = true
		}
	}
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

	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	notify_target, pres := row.GetString("NotifyTarget")
	if !pres {
		return nil
	}

	is_client_connected := self.notification_pool.IsClientConnected(client_id)

	return journal.PushRowsToArtifact(ctx, config_obj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("ClientId", client_id).
			Set("NotifyTarget", notify_target).
			Set("From", self.uuid).
			Set("Connected", is_client_connected)},
		"Server.Internal.Pong", "server", "")
}

func (self *Notifier) ListenForNotification(client_id string) (chan bool, func()) {
	return self.notification_pool.Listen(client_id)
}

func (self *Notifier) CountConnectedClients() uint64 {
	return self.notification_pool.Count()
}

func (self *Notifier) NotifyListener(
	ctx context.Context, config_obj *config_proto.Config,
	id, tag string) error {

	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	// We need to send this ASAP so we do not use an async send.
	notificationsSentCounter.Inc()
	return journal.PushRowsToArtifact(ctx, config_obj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("Tag", tag).
			Set("Target", id)},
		"Server.Internal.Notifications", "server", "",
	)
}

func (self *Notifier) NotifyDirectListener(client_id string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.notification_pool.IsClientConnected(client_id) {
		self.notification_pool.Notify(client_id)
	}
}

func (self *Notifier) NotifyListenerAsync(
	ctx context.Context, config_obj *config_proto.Config, id, tag string) {
	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return
	}

	notificationsSentCounter.Inc()
	journal.PushRowsToArtifactAsync(ctx, config_obj,
		ordereddict.NewDict().
			Set("Tag", tag).
			Set("Target", id),
		"Server.Internal.Notifications")
}

func (self *Notifier) IsClientDirectlyConnected(client_id string) bool {
	self.mu.Lock()
	defer self.mu.Unlock()

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

	// Shortcut if the client is directly connected.
	if self.IsClientDirectlyConnected(client_id) {
		return true
	}

	// No directly connected minions right now, and the client is not
	// connected to us - therefore the client is not available.
	frontend_manager, err := services.GetFrontendManager(config_obj)
	if err != nil {
		return false
	}
	minion_count := frontend_manager.GetMinionCount()
	if minion_count == 0 {
		return false
	}

	// We deem a client connected if the last ping time is within 10 seconds
	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return false
	}

	stats, err := client_info_manager.GetStats(ctx, client_id)
	if err != nil {
		return false
	}

	recent := uint64(time.Now().UnixNano()/1000) - 20*1000000
	if stats.Ping > recent {
		return true
	}

	// Get a unique id for this request.
	id := fmt.Sprintf("IsClientConnected%v", utils.GetId())

	// Send ping to all nodes, they will reply with a
	// notification.
	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return false
	}

	// Channel to be signalled when all responses come back.
	done := make(chan bool)
	self.mu.Lock()
	// Install a tracker to keep track of this request.
	self.client_connection_tracker[id] = &tracker{
		count: minion_count,
		done:  done,
	}
	self.mu.Unlock()

	// Push request immediately for low latency.
	err = journal.PushRowsToArtifact(ctx, config_obj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("ClientId", client_id).
			Set("NotifyTarget", id)},
		"Server.Internal.Ping", "server", "")
	if err != nil {
		return false
	}

	// Now wait here for the reply.
	select {
	case <-done:
		// Signal that all minions indicated if the client was found
		// or not.

	case <-time.After(utils.Jitter(time.Duration(timeout) * time.Second)):
		if timeout > 0 {
			timeoutClientPing.Inc()
		}
		// Nope - not found within the timeout just give up.
	}

	self.mu.Lock()
	tracker := self.client_connection_tracker[id]
	delete(self.client_connection_tracker, id)
	self.mu.Unlock()

	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	return tracker.connected
}
