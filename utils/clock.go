package utils

import (
	"crypto/rand"
	"encoding/binary"
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

// A clock that behaves like a real one with sleeps but can be moved
// forward arbitrarily.
type RealClockWithOffset struct {
	Duration time.Duration
}

func (self RealClockWithOffset) Sleep(d time.Duration) {
	time.Sleep(d)
}

func (self RealClockWithOffset) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

func (self RealClockWithOffset) Now() time.Time {
	return time.Now().Add(self.Duration)
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

	//self.mockNow = self.mockNow.Add(d)
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

var (
	JitterPercent = uint32(10)
)

// Add 10%  of jitter to ensure things dont synchronize
func Jitter(in time.Duration) time.Duration {
	buf := make([]byte, 4)
	_, _ = rand.Read(buf)

	// Random number between 90 to 110 - Small bias but close enough.
	jitter_pc := uint64((binary.BigEndian.Uint32(buf) % (2 * JitterPercent)) - JitterPercent + 100)

	return time.Duration(uint64(in) * jitter_pc / 100)
}
