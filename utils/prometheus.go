package utils

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func GetCounterValue(metric prometheus.Counter) (int64, error) {
	var m = &dto.Metric{}
	if err := metric.Write(m); err != nil {
		return 0, err
	}
	return int64(m.Counter.GetValue()), nil
}

// Starts a goroutine that syncs QPS estimates from a counter to a gauge.
func RegisterQPSCounter(metric prometheus.Counter, gauge prometheus.Gauge) {
	go func() {
		for {
			start := time.Now()
			start_value, _ := GetCounterValue(metric)
			time.Sleep(2 * time.Second)
			end_value, _ := GetCounterValue(metric)
			end := time.Now()
			rate := (end_value - start_value) * 1000000000 / (end.UnixNano() - start.UnixNano())

			gauge.Set(float64(rate))
		}
	}()
}
