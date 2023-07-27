package utils

import (
	"sync"
	"time"
)

var (
	mu sync.Mutex

	mock_time Clock = &RealClock{}
)

func GetTime() Clock {
	mu.Lock()
	defer mu.Unlock()

	return mock_time
}

func MockTime(clock Clock) func() {
	mu.Lock()
	defer mu.Unlock()

	old_time := mock_time
	mock_time = clock

	return func() {
		mu.Lock()
		defer mu.Unlock()

		mock_time = old_time
	}
}

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
	mu      sync.Mutex
	mockNow time.Time
}

func (self *MockClock) Now() time.Time {
	self.mu.Lock()
	defer self.mu.Unlock()
	return self.mockNow
}

func (self *MockClock) Set(t time.Time) {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.mockNow = t
}

// Advance the time and return immediately for sleeps.
func (self *MockClock) After(d time.Duration) <-chan time.Time {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.mockNow = self.mockNow.Add(d)
	res := make(chan time.Time)
	close(res)
	return res
}

func (self *MockClock) Sleep(d time.Duration) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.mockNow = self.mockNow.Add(d)
}

func NewMockClock(now time.Time) *MockClock {
	return &MockClock{
		mockNow: now,
	}
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
