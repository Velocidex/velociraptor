package authenticators

import (
	"context"
	"net/http"

	"github.com/gorilla/csrf"
	"github.com/sirupsen/logrus"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/users"
)

// Implement basic authentication.
type BasicAuthenticator struct{}

// Basic auth does not need any special handlers.
func (self *BasicAuthenticator) AddHandlers(config_obj *config_proto.Config, mux *http.ServeMux) error {
	mux.Handle("/logoff", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, _, ok := r.BasicAuth()
		if !ok {
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		params := r.URL.Query()
		old_username, ok := params["username"]
		if ok && len(old_username) == 1 && old_username[0] != username {
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
		http.Error(w, "authorization failed", http.StatusUnauthorized)
	}))

	return nil
}

func (self *BasicAuthenticator) IsPasswordLess() bool {
	return false
}

func (self *BasicAuthenticator) AuthenticateUserHandler(
	config_obj *config_proto.Config,
	parent http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-CSRF-Token", csrf.Token(r))
		w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)

		username, password, ok := r.BasicAuth()
		if !ok {
			http.Error(w, "Not authorized", http.StatusUnauthorized)
			return
		}

		// Get the full user record with hashes so we can
		// verify it below.
		user_record, err := users.GetUserWithHashes(config_obj, username)
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.Audit)
			logger.WithFields(logrus.Fields{
				"username": username,
				"status":   http.StatusUnauthorized,
			}).Error("Unknown username")

			http.Error(w, "authorization failed", http.StatusUnauthorized)
			return
		}

		// Must have at least reader.
		perm, err := acls.CheckAccess(config_obj, username, acls.READ_RESULTS)
		if !perm || err != nil || user_record.Locked || user_record.Name != username {
			logger := logging.GetLogger(config_obj, &logging.Audit)
			logger.WithFields(logrus.Fields{
				"username": username,
				"status":   http.StatusUnauthorized,
			}).Error("Unauthorized username")

			http.Error(w, "authorization failed", http.StatusUnauthorized)
			return
		}

		if !users.VerifyPassword(user_record, password) {
			logger := logging.GetLogger(config_obj, &logging.Audit)
			logger.WithFields(logrus.Fields{
				"username": username,
				"status":   http.StatusUnauthorized,
			}).Error("Invalid password")

			http.Error(w, "authorization failed", http.StatusUnauthorized)
			return
		}

		// Checking is successfull - user authorized. Here we
		// build a token to pass to the underlying GRPC
		// service with metadata about the user.
		user_info := &api_proto.VelociraptorUser{
			Name: username,
		}

		// Must use json encoding because grpc can not handle
		// binary data in metadata.
		serialized, _ := json.Marshal(user_info)
		ctx := context.WithValue(
			r.Context(), constants.GRPC_USER_CONTEXT, string(serialized))

		// Need to call logging after auth so it can access
		// the USER value in the context.
		GetLoggingHandler(config_obj)(parent).ServeHTTP(
			w, r.WithContext(ctx))
	})
}
