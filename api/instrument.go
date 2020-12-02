package api

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	apiHistorgram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gui_api_latency",
			Help:    "Latency to server API calls.",
			Buckets: prometheus.LinearBuckets(0.01, 0.05, 10),
		},
		[]string{"api"},
	)

	inject_time = 0
)

func Instrument(api string) func() time.Duration {
	timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
		apiHistorgram.WithLabelValues(api).Observe(v)
	}))

	// Instrument a delay in API calls.
	if inject_time > 0 {
		time.Sleep(time.Duration(inject_time) * time.Millisecond)
	}

	return timer.ObserveDuration
}
