package utils

import (
	"context"
	"sync/atomic"
	"time"

	errors "github.com/go-errors/errors"
)

type Gauge interface {
	Inc()
	Dec()
}

type Concurrency struct {
	concurrency chan bool
	counter     Gauge
	timeout     time.Duration
}

func (self *Concurrency) SetGauge(counter Gauge) {
	self.counter = counter
}

func (self *Concurrency) StartConcurrencyControl(ctx context.Context) (func(), error) {
	// Wait here until we have enough room in the concurrency
	// channel.
	select {
	case self.concurrency <- true:
		self.counter.Inc()
		return self.EndConcurrencyControl, nil

	case <-ctx.Done():
		return nil, errors.New("Concurrency: Timed out due to cancellation")

	case <-time.After(self.timeout):
		return nil, errors.New("Timed out in concurrency control")
	}
}

func (self *Concurrency) EndConcurrencyControl() {
	<-self.concurrency
	self.counter.Dec()
}

type NullGauge struct{}

func (NullGauge) Inc() {}
func (NullGauge) Dec() {}

type AtomicGauge struct {
	value int64
}

func (self *AtomicGauge) Inc() {
	atomic.AddInt64(&self.value, 1)
}

func (self *AtomicGauge) Dec() {
	atomic.AddInt64(&self.value, -1)
}

func (self *AtomicGauge) Get() int64 {
	return atomic.LoadInt64(&self.value)
}

func NewConcurrencyControl(size int, timeout time.Duration) *Concurrency {
	return &Concurrency{
		timeout:     timeout,
		counter:     NullGauge{},
		concurrency: make(chan bool, size),
	}
}
