package logging

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/sirupsen/logrus"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
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

func GetUserInfo(ctx context.Context,
	config_obj *api_proto.Config) *api_proto.VelociraptorUser {
	result := &api_proto.VelociraptorUser{}

	userinfo, ok := ctx.Value("USER").(string)
	if ok {
		data := []byte(userinfo)
		err := json.Unmarshal(data, result)
		if err != nil {
			GetLogger(config_obj, &GUIComponent).Error(
				"Unable to Unmarshal USER Token")
		}
	}
	return result
}

func GetLoggingHandler(config_obj *api_proto.Config) func(http.Handler) http.Handler {
	logger := GetLogger(config_obj, &GUIComponent)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &statusRecorder{
				w,
				w.(http.Flusher),
				w.(http.CloseNotifier),
				200}
			defer func() {
				logger.WithFields(
					logrus.Fields{
						"method":     r.Method,
						"url":        r.URL.Path,
						"remote":     r.RemoteAddr,
						"user-agent": r.UserAgent(),
						"status":     rec.status,
						"user": GetUserInfo(
							r.Context(), config_obj).Name,
					}).Info("")
			}()
			next.ServeHTTP(rec, r)
		})
	}
}
