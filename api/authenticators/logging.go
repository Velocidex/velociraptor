package authenticators

import (
	"context"
	"net/http"

	"github.com/sirupsen/logrus"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	api_utils "www.velocidex.com/golang/velociraptor/api/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	http_utils "www.velocidex.com/golang/velociraptor/utils/http"
)

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
		return api_utils.HandlerFunc(next,
			func(w http.ResponseWriter, r *http.Request) {
				rec := &http_utils.StatusRecorder{
					ResponseWriter: w,
					Flusher:        w.(http.Flusher),
					Status:         200}
				defer func() {
					if rec.Status == 500 {
						logger.WithFields(
							logrus.Fields{
								"method":     r.Method,
								"url":        r.URL.Path,
								"remote":     r.RemoteAddr,
								"error":      string(rec.Error),
								"user-agent": r.UserAgent(),
								"status":     rec.Status,
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
								"status":     rec.Status,
								"user": GetUserInfo(
									r.Context(), config_obj).Name,
							}).Info("")
					}
				}()
				next.ServeHTTP(rec, r)
			})
	}
}
