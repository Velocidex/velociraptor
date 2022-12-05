package process

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Accessor for processes.

var (
	processAccessorCurrentOpened = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "accessor_process_current_open",
		Help: "Number of currently opened processes",
	})

	processAccessorTotalOpened = promauto.NewCounter(prometheus.CounterOpts{
		Name: "accessor_process_total_open",
		Help: "Total Number of opened processes",
	})

	processAccessorTotalReadProcessMemory = promauto.NewCounter(prometheus.CounterOpts{
		Name: "accessor_process_total_read_process_memory",
		Help: "Total Number of opened buffers read from process memory",
	})
)
