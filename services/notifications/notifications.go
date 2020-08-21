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
	"sync"

	"github.com/Velocidex/ordereddict"
	"github.com/pkg/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/notifications"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
)

type Notifier struct {
	pool_mu           sync.Mutex
	notification_pool *notifications.NotificationPool
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

	self := &Notifier{}

	self.pool_mu.Lock()
	defer self.pool_mu.Unlock()

	if config_obj.Datastore == nil {
		return errors.New("Filestore not configured")
	}

	if self.notification_pool != nil {
		self.notification_pool.Shutdown()
	}

	self.notification_pool = notifications.NewNotificationPool()

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> the notification service.")

	// Watch the journal.
	events, cancel := services.GetJournal().Watch("Server.Internal.Notifications")

	wg.Add(1)
	go func() {
		defer cancel()
		defer wg.Done()

		defer func() {
			self.pool_mu.Lock()
			defer self.pool_mu.Unlock()

			self.notification_pool.Shutdown()
			self.notification_pool = nil
		}()

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

				self.pool_mu.Lock()
				if target == "All" {
					self.notification_pool.NotifyAll()
				} else {
					self.notification_pool.Notify(target)
				}
				self.pool_mu.Unlock()
			}
		}
	}()

	services.RegisterNotifier(self)

	return nil
}

func (self *Notifier) ListenForNotification(client_id string) (chan bool, func()) {
	self.pool_mu.Lock()
	defer self.pool_mu.Unlock()

	if self.notification_pool == nil {
		self.notification_pool = notifications.NewNotificationPool()
	}

	return self.notification_pool.Listen(client_id)
}

func (self *Notifier) NotifyAllListeners(config_obj *config_proto.Config) error {
	path_manager := result_sets.NewArtifactPathManager(
		config_obj, "server" /* client_id */, "", "Server.Internal.Notifications")

	return services.GetJournal().PushRows(path_manager,
		[]*ordereddict.Dict{ordereddict.NewDict().Set("Target", "All")})
}

func (self *Notifier) NotifyListener(config_obj *config_proto.Config, id string) error {
	path_manager := result_sets.NewArtifactPathManager(
		config_obj, "server" /* client_id */, "", "Server.Internal.Notifications")

	return services.GetJournal().PushRows(path_manager,
		[]*ordereddict.Dict{ordereddict.NewDict().Set("Target", id)})
}

// TODO: Make this work on all frontends.
func (self *Notifier) IsClientConnected(client_id string) bool {
	self.pool_mu.Lock()
	defer self.pool_mu.Unlock()

	if self.notification_pool == nil {
		self.notification_pool = notifications.NewNotificationPool()
	}

	return self.notification_pool.IsClientConnected(client_id)
}
