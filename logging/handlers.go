package logging

import (
	"net/http"

	"www.velocidex.com/golang/velociraptor/config"
)

// Record the status of the request so we can log it.
type statusRecorder struct {
	http.ResponseWriter
	http.Flusher
	http.CloseNotifier
	status int
}

func (rec *statusRecorder) WriteHeader(code int) {
	rec.status = code
	rec.ResponseWriter.WriteHeader(code)
}

func GetLoggingHandler(config_obj *config.Config) func(http.Handler) http.Handler {
	logger := NewLogger(config_obj)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &statusRecorder{
				w,
				w.(http.Flusher),
				w.(http.CloseNotifier),
				200}
			defer func() {
				logger.Info(
					"%s %s %s %s %d",
					r.Method,
					r.URL.Path,
					r.RemoteAddr,
					r.UserAgent(),
					rec.status)
			}()
			next.ServeHTTP(rec, r)
		})
	}
}
