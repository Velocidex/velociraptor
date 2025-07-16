package notifications

import (
	"context"
	"sync"
	"testing"
	"time"

	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func TestNotifications(t *testing.T) {
	client_id := "C.1234"
	ctx := context.Background()
	wg := &sync.WaitGroup{}
	notifier := NewNotificationPool(ctx, wg)

	triggered := &utils.Counter{}

	go func() {
		notification, cancel := notifier.Listen(client_id)
		defer cancel()

		select {
		case <-notification:
			triggered.Inc()
		case <-time.After(500 * time.Millisecond):
		}
	}()

	// Wait until the notification is in place.
	vtesting.WaitUntil(time.Second, t, func() bool {
		return len(notifier.ListClients()) > 0
	})

	assert.Equal(t, 0, triggered.Get())
	assert.Equal(t, []string{client_id}, notifier.ListClients())

	// Now trigger the notification and wait for the triggered flag to increase.
	notifier.Notify(client_id)

	vtesting.WaitUntil(time.Second, t, func() bool {
		return triggered.Get() == 1
	})
}

// If notifications arrive a little out of order this is ok.
func TestNotificationsOutOfOrder(t *testing.T) {
	client_id := "C.1234"
	ctx := context.Background()
	wg := &sync.WaitGroup{}
	notifier := NewNotificationPool(ctx, wg)

	triggered := &utils.Counter{}

	// Trigger the notification before the listener is in place.
	notifier.Notify(client_id)

	// Watch for notification.
	go func() {
		notification, cancel := notifier.Listen(client_id)
		defer cancel()

		select {
		case <-notification:
			triggered.Inc()
		case <-time.After(500 * time.Millisecond):
		}
	}()

	// Should have triggered
	vtesting.WaitUntil(1000*time.Second, t, func() bool {
		return triggered.Get() == 1
	})
}

// When multiple notifications occur very quickly and the listener
// does not have time to re-establish listening channels, the
func TestMultipleNotificationsToDelayedListener(t *testing.T) {
	client_id := "C.1234"
	ctx := context.Background()
	wg := &sync.WaitGroup{}
	notifier := NewNotificationPool(ctx, wg)

	triggered := &utils.Counter{}
	notification_count := &utils.Counter{}
	time_out := &utils.Counter{}

	go func() {
		// Wait here for multiple notifications and accumulate them.
		for {
			// The notifier is not active now.
			assert.Equal(t, false, notifier.IsClientConnected(client_id))

			notification, cancel := notifier.Listen(client_id)

			// The notifier is active now since we are listening to it.
			assert.Equal(t, true, notifier.IsClientConnected(client_id))

			select {
			case <-notification:
				cancel()
				triggered.Inc()

				// Exit after a short time.
			case <-time.After(500 * time.Millisecond):
				time_out.Inc()
				return
			}
		}
	}()

	// Wait until the notification is in place.
	vtesting.WaitUntil(time.Second, t, func() bool {
		return len(notifier.ListClients()) > 0
	})

	assert.Equal(t, 0, triggered.Get())

	// Now trigger the notifications very quickly.
	for i := 0; i < 100; i++ {
		notifier.Notify(client_id)
		notification_count.Inc()
		time.Sleep(time.Millisecond)
	}

	// Wait here until the goroutine is done.
	vtesting.WaitUntil(4*time.Second, t, func() bool {
		return time_out.Get() > 0 && triggered.Get() == 2
	})

	assert.Equal(t, 2, triggered.Get())
	assert.Equal(t, 100, notification_count.Get())
}
