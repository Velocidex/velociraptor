/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/sirupsen/logrus"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

// Record the status of the request so we can log it.
type statusRecorder struct {
	http.ResponseWriter
	http.Flusher
	http.CloseNotifier
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

	userinfo, ok := ctx.Value("USER").(string)
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
				w.(http.CloseNotifier),
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
