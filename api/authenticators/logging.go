package authenticators

import (
	"context"
	"net/http"

	"github.com/sirupsen/logrus"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
)

// Record the status of the request so we can log it.
type statusRecorder struct {
	http.ResponseWriter
	http.Flusher
	status int
	error  []byte
}

func (self *statusRecorder) WriteHeader(code int) {
	self.status = code
	self.ResponseWriter.WriteHeader(code)
}

func (self *statusRecorder) Write(buf []byte) (int, error) {
	if self.status == 500 {
		self.error = buf
	}

	return self.ResponseWriter.Write(buf)
}

func GetUserInfo(ctx context.Context,
	config_obj *config_proto.Config) *api_proto.VelociraptorUser {
	result := &api_proto.VelociraptorUser{}

	userinfo, ok := ctx.Value(constants.GRPC_USER_CONTEXT).(string)
	if ok {
		data := []byte(userinfo)
		err := json.Unmarshal(data, result)
		if err != nil {
			logging.GetLogger(config_obj, &logging.GUIComponent).Error(
				"Unable to Unmarshal USER Token")
		}
	}
	return result
}

func GetLoggingHandler(config_obj *config_proto.Config) func(http.Handler) http.Handler {
	logger := logging.GetLogger(config_obj, &logging.GUIComponent)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &statusRecorder{
				w,
				w.(http.Flusher),
				200, nil}
			defer func() {
				if rec.status == 500 {
					logger.WithFields(
						logrus.Fields{
							"method":     r.Method,
							"url":        r.URL.Path,
							"remote":     r.RemoteAddr,
							"error":      string(rec.error),
							"user-agent": r.UserAgent(),
							"status":     rec.status,
							"user": GetUserInfo(
								r.Context(), config_obj).Name,
						}).Error("")

				} else {
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
				}
			}()
			next.ServeHTTP(rec, r)
		})
	}
}
