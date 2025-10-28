package ntfs

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	NTFSHistorgram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ntfs_accessor",
			Help:    "Latency to access file accessor.",
			Buckets: prometheus.LinearBuckets(0.01, 0.05, 10),
		},
		[]string{"action"},
	)
)

func Instrument(access_type string) func() time.Duration {
	timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
		NTFSHistorgram.WithLabelValues(access_type).Observe(v)
	}))

	return timer.ObserveDuration
}
