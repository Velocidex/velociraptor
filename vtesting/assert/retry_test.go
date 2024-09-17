package assert

import (
	"testing"
	"time"
)

func TestRetry(t *testing.T) {
	i := 0
	tests := []bool{false, false, true}

	True(t, Retry(t, 4, time.Millisecond, func(r *R) {
		True(r, tests[i])
		i++
	}))
}
