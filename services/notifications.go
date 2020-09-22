package services

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
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

var (
	notification_mu sync.Mutex
	g_notification  Notifier = nil
)

func GetNotifier() Notifier {
	notification_mu.Lock()
	defer notification_mu.Unlock()

	return g_notification
}

func RegisterNotifier(n Notifier) {
	notification_mu.Lock()
	defer notification_mu.Unlock()

	g_notification = n
}

type Notifier interface {
	// Receives a channel which will be closed when a notification
	// occurs. Callers may block on this channel until the event
	// occurs, after which they will need to re-listen to the
	// event again. Note that there is an inherent race condition
	// between the time an event is processed and a new event
	// channel is obtained. For reliable event notification use
	// the Journal service.
	ListenForNotification(id string) (chan bool, func())

	// Send a notification to everyone - this is a global event
	// directed at everyone.
	NotifyAllListeners(config_obj *config_proto.Config) error

	// Send a notification to a specific listener based on its id
	// that was registered above.
	NotifyListener(config_obj *config_proto.Config, id string) error

	// Check if there is someone listening for the specified
	// id. Currently this only works on one frontend.
	IsClientConnected(client_id string) bool
}
