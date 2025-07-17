package utils

import (
	"context"
	"testing"
	"time"

	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func TestThrottler(t *testing.T) {
	count := 0
	spin := 0

	// This gives a slot every 100ms
	throttler := NewThrottlerWithDuration(100 * time.Millisecond)

	// Run the test for 500mb - we should be able to count at least 3 times.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

main_loop:
	for {
		spin++
		select {
		case <-ctx.Done():
			break main_loop
		default:
			if throttler.Ready() {
				count++
			}
		}
	}

	assert.True(t, count >= 3)
	assert.True(t, spin > 20)
}
