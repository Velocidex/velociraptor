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
	"regexp"
	"sync"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/notifications"
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

	if self.notification_pool != nil {
		self.notification_pool.Shutdown()
	}

	self.notification_pool = notifications.NewNotificationPool()

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> the notification service.")

	// Watch the journal.
	journal, err := services.GetJournal()
	if err != nil {
		return err
	}
	events, cancel := journal.Watch(ctx, "Server.Internal.Notifications")

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

func (self *Notifier) ListenForNotification(client_id string) (chan bool, func()) {
	self.pool_mu.Lock()
	defer self.pool_mu.Unlock()

	if self.notification_pool == nil {
		self.notification_pool = notifications.NewNotificationPool()
	}

	return self.notification_pool.Listen(client_id)
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

func (self *Notifier) NotifyByRegex(config_obj *config_proto.Config, regex string) error {
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

// TODO: Make this work on all frontends.
func (self *Notifier) IsClientConnected(client_id string) bool {
	self.pool_mu.Lock()
	defer self.pool_mu.Unlock()

	if self.notification_pool == nil {
		self.notification_pool = notifications.NewNotificationPool()
	}

	return self.notification_pool.IsClientConnected(client_id)
}
