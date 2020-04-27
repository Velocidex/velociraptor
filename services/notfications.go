package services

import (
	"sync"

	"github.com/Velocidex/ordereddict"
	"github.com/pkg/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/notifications"
)

var (
	pool_mu           sync.Mutex
	notification_pool *notifications.NotificationPool
)

// The notifier service watches for events from
// Server.Internal.Notifications and notifies the notification pool in
// the current process. This allows multiprocess communication as the
// notifications may arrive from other frontend processes through the
// journal service.
func startNotificationService(
	config_obj *config_proto.Config,
	notifier *notifications.NotificationPool) error {
	pool_mu.Lock()
	defer pool_mu.Unlock()

	if notifier == nil {
		return errors.New("Notifier must be specified.")
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("Starting the notification service.")

	notification_pool = notifier
	go func() {
		events, cancel := GetJournal().Watch("Server.Internal.Notifications")
		defer cancel()

		for event := range events {
			target, ok := event.GetString("Target")
			if !ok {
				continue
			}

			if target == "All" {
				notification_pool.NotifyAll()
			} else {
				notification_pool.Notify(target)
			}
		}
	}()

	return nil
}

func NotifyAll() error {
	return GetJournal().PushRows("Server.Internal.Notifications", "", "server",
		[]*ordereddict.Dict{ordereddict.NewDict().Set("Target", "All")})
}

func NotifyClient(client_id string) error {
	return GetJournal().PushRows("Server.Internal.Notifications", "", "server",
		[]*ordereddict.Dict{ordereddict.NewDict().Set("Target", client_id)})
}
