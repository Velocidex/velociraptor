package datastore

import (
	"os"
	"strconv"
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
		[]string{"tag", "action", "datastore"},
	)

	// Simulate running on a very slow filesystem (EFS)
	inject_time = 0
)

func Instrument(access_type, datastore string,
	path_spec api.DSPathSpec) func() time.Duration {

	tag := path_spec.Tag()
	if tag == "" {
		tag = "Generic"
	}

	timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
		DatastoreHistorgram.WithLabelValues(tag, access_type, datastore).Observe(v)
	}))

	return timer.ObserveDuration
}

func InstrumentWithDelay(
	access_type, datastore string, path_spec api.DSPathSpec) func() time.Duration {

	tag := path_spec.Tag()
	if tag == "" {
		tag = "Generic"
	}

	timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
		DatastoreHistorgram.WithLabelValues(tag, access_type, datastore).Observe(v)
	}))

	// Instrument a delay in API calls.
	if inject_time > 0 {
		time.Sleep(time.Duration(inject_time) * time.Millisecond)
	}

	return timer.ObserveDuration
}

func init() {
	delay_str, pres := os.LookupEnv("VELOCIRAPTOR_SLOW_FILESYSTEM")
	if pres {
		delay, err := strconv.Atoi(delay_str)
		if err == nil {
			inject_time = delay
		}
	}
}
