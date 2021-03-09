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

func NewThrottler(connections_per_second uint64) *Throttler {
	duration := time.Duration(1000/connections_per_second) * time.Millisecond
	return &Throttler{
		ticker: time.Tick(duration),
		done:   make(chan bool),
	}
}
