package services

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
	ListenForNotification(client_id string) (chan bool, func())
	NotifyAllListeners(config_obj *config_proto.Config) error
	NotifyListener(config_obj *config_proto.Config, id string) error
	IsClientConnected(client_id string) bool
}
