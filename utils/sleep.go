package utils

import (
	"context"
	"time"
)

func SleepWithCtx(ctx context.Context,
	duration time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(duration):
		return true
	}
}
