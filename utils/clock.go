package utils

import (
	"sync"
	"time"
)

type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
	Sleep(d time.Duration)
}

type RealClock struct{}

func (self RealClock) Sleep(d time.Duration) {
	time.Sleep(d)
}

func (self RealClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

func (self RealClock) Now() time.Time {
	return time.Now()
}

type MockClock struct {
	MockNow  time.Time
	duration time.Duration
}

func (self MockClock) Now() time.Time {
	return self.MockNow
}

func (self MockClock) After(d time.Duration) <-chan time.Time {
	return time.After(self.duration)
}

func (self MockClock) Sleep(d time.Duration) {
	time.Sleep(self.duration)
}

// A clock that increments each time someone calls Now()
type IncClock struct {
	mu      sync.Mutex
	NowTime int64
}

func (self *IncClock) Now() time.Time {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.NowTime++
	return time.Unix(self.NowTime, 0)
}

func (self *IncClock) After(d time.Duration) <-chan time.Time {
	return time.After(0)
}

func (self *IncClock) Sleep(d time.Duration) {
	time.Sleep(0)
}
