package server

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpErrorStatusCounters = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "frontend_http_status",
			Help: "Count of various http status.",
		},
		[]string{"status"},
	)
)

func RecordHTTPStats(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{
			w,
			w.(http.Flusher),
			200, nil}

		next.ServeHTTP(rec, r)
		status := fmt.Sprintf("%v", rec.status)
		httpErrorStatusCounters.WithLabelValues(status).Inc()
	})
}
