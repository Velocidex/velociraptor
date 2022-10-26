package utils

import (
	"context"
	"time"

	errors "github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	concurrencyControl = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "client_comms_concurrency",
		Help: "The total number of currently executing client receive operations",
	})
)

type Concurrency struct {
	concurrency chan bool
	timeout     time.Duration
}

func (self *Concurrency) StartConcurrencyControl(ctx context.Context) (func(), error) {
	// Wait here until we have enough room in the concurrency
	// channel.
	select {
	case self.concurrency <- true:
		concurrencyControl.Inc()
		return self.EndConcurrencyControl, nil

	case <-ctx.Done():
		return nil, errors.New("Concurrency: Timed out due to cancellation")

	case <-time.After(self.timeout):
		return nil, errors.New("Timed out in concurrency control")
	}
}

func (self *Concurrency) EndConcurrencyControl() {
	<-self.concurrency
	concurrencyControl.Dec()
}

func NewConcurrencyControl(size int, timeout time.Duration) *Concurrency {
	return &Concurrency{
		timeout:     timeout,
		concurrency: make(chan bool, size),
	}
}
