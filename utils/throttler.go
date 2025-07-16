package utils

import "time"

type Throttler struct {
	ticker chan time.Time
	done   chan bool
}

func (self *Throttler) Ready() bool {
	select {
	case <-self.ticker:
		return true
	default:
		return false
	}
}

func (self *Throttler) Wait() {
	select {
	case <-self.ticker:
		return
	}
}

func (self *Throttler) Close() {
	close(self.done)
}

// This throttler is used to limit the number of connections per
// second. When performing a hunt it may be possible that all clients
// attempt to connect to the server at the same time, significantly
// increasing network load on the server and limiting processing
// capacity. We use this throttler to control this and reject
// connections as a load shedding strategy. The rejected clients will
// automatically back off and attempt to reconnect in a short time.
func NewThrottler(connections_per_second uint64) *Throttler {
	if connections_per_second == 0 || connections_per_second > 1000 {
		return NewThrottlerWithDuration(0)
	}

	duration := time.Duration(
		1000000/connections_per_second) * time.Microsecond
	return NewThrottlerWithDuration(duration)
}

func NewThrottlerWithDuration(duration time.Duration) *Throttler {
	if duration == 0 {
		result := &Throttler{
			ticker: make(chan time.Time),
			done:   make(chan bool),
		}

		close(result.ticker)
		return result
	}

	result := &Throttler{
		// Have some buffering so we can spike QPS temporarily
		// for 10 seconds
		ticker: make(chan time.Time, 10),
		done:   make(chan bool),
	}

	go func() {
		defer close(result.ticker)

		ticker := time.NewTicker(duration)
		defer ticker.Stop()

		for {
			select {
			case <-result.done:
				return

			case value := <-ticker.C:
				result.ticker <- value
			}
		}
	}()

	return result
}
