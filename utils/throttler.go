package utils

import "time"

type Throttler struct {
	ticker <-chan time.Time
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

// This throttler is used to limit the number of connections per
// second. When performing a hunt it may be possible that all clients
// attempt to conenct to the server at the same time, significantly
// increasing network load on the server and limiting processing
// capacity. We use this throttler to control this and reject
// connections as a load shedding strategy. The rejected clients will
// automatically back off and attempt to reconnect in a short time.
func NewThrottler(connections_per_second uint64) *Throttler {
	duration := time.Duration(1000000/connections_per_second) * time.Microsecond
	return &Throttler{
		ticker: time.Tick(duration),
		done:   make(chan bool),
	}
}
