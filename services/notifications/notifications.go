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
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/notifications"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/journal"
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
		defer cancel()
		defer wg.Done()
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

				if target == "Regex" {
					regex_str, ok := event.GetString("Regex")
					if ok {
						regex, err := regexp.Compile(regex_str)
						if err != nil {
							logger.Error("Notification service: "+
								"Unable to compiler regex '%v': %v\n",
								regex_str, err)
							continue
						}
						self.notification_pool.NotifyByRegex(config_obj, regex)
					}

				} else if target == "All" {
					self.notification_pool.NotifyAll(config_obj)
				} else {
					self.notification_pool.Notify(target)
				}
			}
		}
	}()

	services.RegisterNotifier(self)

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

	notify_target, pres := row.GetString("NotifyTarget")
	if !pres {
		return nil
	}

	// Notify the target of the Ping.
	return self.NotifyListener(config_obj, notify_target)
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

func (self *Notifier) NotifyAllListeners(config_obj *config_proto.Config) error {
	journal, err := services.GetJournal()
	if err != nil {
		return err
	}

	return journal.PushRowsToArtifact(config_obj,
		[]*ordereddict.Dict{ordereddict.NewDict().Set("Target", "All")},
		"Server.Internal.Notifications", "server", "",
	)
}

func (self *Notifier) NotifyByRegex(
	config_obj *config_proto.Config, regex string) error {
	journal, err := services.GetJournal()
	if err != nil {
		return err
	}

	return journal.PushRowsToArtifact(config_obj,
		[]*ordereddict.Dict{ordereddict.NewDict().Set("Target", "Regex").
			Set("Regex", regex)},
		"Server.Internal.Notifications", "server", "",
	)
}

func (self *Notifier) NotifyListener(config_obj *config_proto.Config, id string) error {
	journal, err := services.GetJournal()
	if err != nil {
		return err
	}

	return journal.PushRowsToArtifact(config_obj,
		[]*ordereddict.Dict{ordereddict.NewDict().Set("Target", id)},
		"Server.Internal.Notifications", "server", "",
	)
}

func (self *Notifier) IsClientConnected(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string, timeout int) bool {

	// Get a unique ID
	atomic.StoreUint64(&self.idx, self.idx+1)

	// Watch for Ping replies on this notification.
	id := fmt.Sprintf("Notify%v", self.idx)
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

	// Now wait here for the reply.
	select {
	case <-done:
		// Client is found!
		return true

	case <-time.After(time.Duration(timeout) * time.Second):
		// Nope - not found within the timeout.
		return false
	}
}
