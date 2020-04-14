package services

import (
	"sync"

	"www.velocidex.com/golang/velociraptor/notifications"
)

var (
	pool_mu           sync.Mutex
	notification_pool *notifications.NotificationPool
)

// For now very simple - the notifier service simply calls on the
// notfication pool because there is only a single frontend. In future
// manage notifications for multiple frontends.
func startNotificationService(notifier *notifications.NotificationPool) error {
	pool_mu.Lock()
	defer pool_mu.Unlock()

	notification_pool = notifier

	return nil
}

func NotifyAll() error {
	pool_mu.Lock()
	defer pool_mu.Unlock()

	if notification_pool != nil {
		notification_pool.NotifyAll()
	}
	return nil
}

func NotifyClient(client_id string) error {
	pool_mu.Lock()
	defer pool_mu.Unlock()

	if notification_pool != nil {
		notification_pool.Notify(client_id)
	}
	return nil
}
