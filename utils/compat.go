package utils

import (
	"context"
	"time"
)

// Redirector for new context functions. May be replaced for building
// with old Golang.
func WithTimeoutCause(ctx context.Context, duration time.Duration, err error) (
	context.Context, func()) {
	return context.WithTimeoutCause(ctx, duration, err)
}

func Cause(ctx context.Context) error {
	return context.Cause(ctx)
}
