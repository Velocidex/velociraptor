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

package services

import (
	"context"
	"sync"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/notifications"
	"www.velocidex.com/golang/velociraptor/result_sets"
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
func StartNotificationService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {
	pool_mu.Lock()
	defer pool_mu.Unlock()

	if notification_pool != nil {
		notification_pool.Shutdown()
	}

	notification_pool = notifications.NewNotificationPool()

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("Starting the notification service.")

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			pool_mu.Lock()
			defer pool_mu.Unlock()

			notification_pool.Shutdown()
			notification_pool = nil
		}()

		events, cancel := GetJournal().Watch("Server.Internal.Notifications")
		defer cancel()

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

				pool_mu.Lock()
				if target == "All" {
					notification_pool.NotifyAll()
				} else {
					notification_pool.Notify(target)
				}
				pool_mu.Unlock()
			}
		}
	}()

	return nil
}

func ListenForNotification(client_id string) (chan bool, func()) {
	pool_mu.Lock()
	defer pool_mu.Unlock()

	if notification_pool == nil {
		notification_pool = notifications.NewNotificationPool()
	}

	return notification_pool.Listen(client_id)
}

func NotifyAllListeners(config_obj *config_proto.Config) error {
	path_manager := result_sets.NewArtifactPathManager(
		config_obj, "server" /* client_id */, "", "Server.Internal.Notifications")

	return GetJournal().PushRows(path_manager,
		[]*ordereddict.Dict{ordereddict.NewDict().Set("Target", "All")})
}

func NotifyListener(config_obj *config_proto.Config, id string) error {
	path_manager := result_sets.NewArtifactPathManager(
		config_obj, "server" /* client_id */, "", "Server.Internal.Notifications")

	return GetJournal().PushRows(path_manager,
		[]*ordereddict.Dict{ordereddict.NewDict().Set("Target", id)})
}

// TODO: Make this work on all frontends.
func IsClientConnected(client_id string) bool {
	pool_mu.Lock()
	defer pool_mu.Unlock()

	if notification_pool == nil {
		notification_pool = notifications.NewNotificationPool()
	}

	return notification_pool.IsClientConnected(client_id)
}
