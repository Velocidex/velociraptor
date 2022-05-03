package api

import (
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	FilestoreHistorgram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "filestore_latency",
			Help:    "Latency to access filestore.",
			Buckets: prometheus.LinearBuckets(0.01, 0.05, 10),
		},
		[]string{"tag", "action", "datastore"},
	)

	// Simulate running on a very slow filesystem (EFS)
	clock_mu    sync.Mutex
	inject_time = 0

	g_clock utils.Clock = &utils.RealClock{}
)

func Clock() utils.Clock {
	clock_mu.Lock()
	defer clock_mu.Unlock()

	return g_clock
}

// Used by tests to force a delay (ms)
func SetFilestoreDelay(delay int) {
	inject_time = delay
}

// Tests use this to install a new clock and latency delays.
func InstallClockForTests(clock utils.Clock, delay int) func() {
	clock_mu.Lock()
	defer clock_mu.Unlock()

	old_delay := inject_time
	old_clock := g_clock

	inject_time = delay
	g_clock = clock

	return func() {
		clock_mu.Lock()
		defer clock_mu.Unlock()

		g_clock = old_clock
		inject_time = old_delay
	}
}

func Instrument(access_type, datastore string, path_spec FSPathSpec) func() time.Duration {
	var tag string
	if path_spec != nil {
		tag = path_spec.Tag()
	}
	if tag == "" {
		tag = "Generic"
	}

	// Mark the start of the time.
	start := Clock().Now()

	// When this func is called we calculate the time difference and
	// observe it into the histogram.
	return func() time.Duration {
		d := Clock().Now().Sub(start)
		FilestoreHistorgram.WithLabelValues(
			tag, access_type, datastore).Observe(d.Seconds())
		return d
	}
}

func InstrumentWithDelay(
	access_type, datastore string, path_spec FSPathSpec) func() time.Duration {

	var tag string
	if path_spec != nil {
		tag = path_spec.Tag()
	}
	if tag == "" {
		tag = "Generic"
	}

	// Mark the start of the time.
	start := Clock().Now()

	// Instrument a delay in API calls.
	if inject_time > 0 {
		Clock().Sleep(time.Duration(inject_time) * time.Millisecond)
	}

	// When this func is called we calculate the time difference and
	// observe it into the histogram.
	return func() time.Duration {
		d := Clock().Now().Sub(start)
		FilestoreHistorgram.WithLabelValues(
			tag, access_type, datastore).Observe(d.Seconds())
		return d
	}
}

func init() {
	// Allow the delay to be specified by the env var.
	delay_str, pres := os.LookupEnv("VELOCIRAPTOR_SLOW_FILESYSTEM")
	if pres {
		delay, err := strconv.Atoi(delay_str)
		if err == nil {
			SetFilestoreDelay(delay)
		}
	}
}
