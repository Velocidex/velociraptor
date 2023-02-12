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
	"context"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func GetNotifier(config_obj *config_proto.Config) (Notifier, error) {
	org_manager, err := GetOrgManager()
	if err != nil {
		return nil, err
	}

	return org_manager.Services(config_obj.OrgId).Notifier()
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

	// Send a notification to a specific listener based on its id
	// that was registered above.
	NotifyListener(ctx context.Context, config_obj *config_proto.Config,
		id, tag string) error

	// Notify a directly connected listener.
	NotifyDirectListener(id string)

	// Notify in the near future - no guarantee of delivery.
	NotifyListenerAsync(ctx context.Context,
		config_obj *config_proto.Config, id, tag string)

	// Check if there is someone listening for the specified id. This
	// method queries all minion nodes to check if the client is
	// connected anywhere - It may take up to 2 seconds to find out.
	IsClientConnected(ctx context.Context,
		config_obj *config_proto.Config,
		client_id string, timeout int) bool

	// Returns a list of all clients directly connected at present.
	ListClients() []string

	CountConnectedClients() uint64

	// Check only the current node if the client is connected.
	IsClientDirectlyConnected(client_id string) bool
}
