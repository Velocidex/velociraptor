package datastore

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

var (
	DatastoreHistorgram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "datastore_latency",
			Help:    "Latency to access datastore.",
			Buckets: prometheus.LinearBuckets(0.01, 0.05, 10),
		},
		[]string{"tag", "action"},
	)

	inject_time = 0
)

func Instrument(access_type string, path_spec api.DSPathSpec) func() time.Duration {
	tag := path_spec.Tag()
	if tag == "" {
		// fmt.Printf("%v: %v\n", access_type, path_spec.AsClientPath())
		tag = "Generic"
	}

	timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
		DatastoreHistorgram.WithLabelValues(tag, access_type).Observe(v)
	}))

	// Instrument a delay in API calls.
	if inject_time > 0 {
		time.Sleep(time.Duration(inject_time) * time.Millisecond)
	}

	return timer.ObserveDuration
}
