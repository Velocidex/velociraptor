package server

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	api_utils "www.velocidex.com/golang/velociraptor/api/utils"
	http_utils "www.velocidex.com/golang/velociraptor/utils/http"
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
	return api_utils.HandlerFunc(nil,
		func(w http.ResponseWriter, r *http.Request) {
			// Ignore Websocket connections
			if is_ws_connection(r) {
				next.ServeHTTP(w, r)
				return
			}

			rec := &http_utils.StatusRecorder{
				ResponseWriter: w,
				Flusher:        w.(http.Flusher),
				Status:         200}

			next.ServeHTTP(rec, r)
			status := fmt.Sprintf("%v", rec.Status)
			httpErrorStatusCounters.WithLabelValues(status).Inc()
		})
}
