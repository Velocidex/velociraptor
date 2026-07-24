package utils

import (
	"context"
	"time"
)

// Sleeps for the duration or if th context is cancelled.  Returns
// true if the sleep was completed successfuly, false if the sleep was
// cut short due to the cancellation.
func SleepWithCtx(ctx context.Context, duration time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(duration):
		return true
	}
}
