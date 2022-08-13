package utils

import (
	"context"
	"time"
)

func SleepWithCtx(ctx context.Context,
	duration time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(duration):
	}
}
