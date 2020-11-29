package utils

import (
	"context"
	"time"

	errors "github.com/pkg/errors"
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

func (self *Concurrency) StartConcurrencyControl(ctx context.Context) error {
	// Wait here until we have enough room in the concurrency
	// channel.
	select {
	case self.concurrency <- true:
		concurrencyControl.Inc()
		return nil

	case <-ctx.Done():
		return errors.New("Timed out")

	case <-time.After(self.timeout):
		return errors.New("Timed out")
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
