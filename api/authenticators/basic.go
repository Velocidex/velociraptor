package authenticators

import (
	"context"
	"net/http"

	"github.com/Velocidex/ordereddict"
	"github.com/gorilla/csrf"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	api_utils "www.velocidex.com/golang/velociraptor/api/utils"
	utils "www.velocidex.com/golang/velociraptor/api/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
)

// Implement basic authentication.
type BasicAuthenticator struct {
	config_obj *config_proto.Config
}

// Basic auth does not need any special handlers.
func (self *BasicAuthenticator) AddHandlers(mux *api_utils.ServeMux) error {
	return nil
}

func (self *BasicAuthenticator) AddLogoff(mux *api_utils.ServeMux) error {
	mux.Handle(api_utils.GetBasePath(self.config_obj, "/app/logoff.html"),
		IpFilter(self.config_obj,
			api_utils.HandlerFunc(nil,
				func(w http.ResponseWriter, r *http.Request) {
					username, _, ok := r.BasicAuth()
					if !ok {
						w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
						http.Error(w, "authorization failed", http.StatusUnauthorized)
						return
					}

					// The previous username is given as a query parameter.
					params := r.URL.Query()
					old_username, ok := params["username"]
					if ok && len(old_username) == 1 && old_username[0] != username {
						// Authenticated as someone else.
						http.Redirect(w, r, utils.Homepage(self.config_obj),
							http.StatusTemporaryRedirect)
						return
					}

					w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
					http.Error(w, "authorization failed", http.StatusUnauthorized)
				})))

	return nil
}

func (self *BasicAuthenticator) IsPasswordLess() bool {
	return false
}

func (self *BasicAuthenticator) RequireClientCerts() bool {
	return false
}

func (self *BasicAuthenticator) AuthRedirectTemplate() string {
	return ""
}

func (self *BasicAuthenticator) AuthenticateUserHandler(
	parent http.Handler,
	permission acls.ACL_PERMISSION,
) http.Handler {

	logger := GetLoggingHandler(self.config_obj)(parent)

	return api_utils.HandlerFunc(parent,
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-CSRF-Token", csrf.Token(r))
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)

			username, password, ok := r.BasicAuth()
			if !ok {
				http.Error(w, "Not authorized", http.StatusUnauthorized)
				return
			}

			// Get the full user record with hashes so we can
			// verify it below.
			users_manager := services.GetUserManager()
			user_record, err := users_manager.GetUserWithHashes(r.Context(),
				username, username)
			if err != nil {
				err := services.LogAudit(r.Context(),
					self.config_obj, username, "Unknown username",
					ordereddict.NewDict().
						Set("remote", r.RemoteAddr).
						Set("status", http.StatusUnauthorized))
				if err != nil {
					logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
					logger.Error("Unknown username %v %v", username, r.RemoteAddr)
				}
				http.Error(w, "authorization failed", http.StatusUnauthorized)
				return
			}

			ok, err = users_manager.VerifyPassword(r.Context(),
				user_record.Name, user_record.Name, password)
			if !ok || err != nil {
				err := services.LogAudit(r.Context(),
					self.config_obj, user_record.Name, "Invalid password",
					ordereddict.NewDict().
						Set("remote", r.RemoteAddr).
						Set("status", http.StatusUnauthorized))

				// If we cant emit an audit log, log to regular logging.
				if err != nil {
					logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
					logger.Error("Invalid Password %v %v", user_record.Name, r.RemoteAddr)
				}

				http.Error(w, "authorization failed", http.StatusUnauthorized)
				return
			}

			// Does the user have access to the specified org?
			err = CheckOrgAccess(self.config_obj, r, user_record, permission)
			if err != nil {
				err1 := services.LogAudit(r.Context(),
					self.config_obj, user_record.Name, "User Unauthorized for Org",
					ordereddict.NewDict().
						Set("err", err.Error()).
						Set("remote", r.RemoteAddr).
						Set("status", http.StatusUnauthorized))
				if err1 != nil {
					logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
					logger.Error("CheckOrgAccess LogAudit: User Unauthorized for Org %v %v",
						user_record.Name, r.RemoteAddr)
				}

				// Return status forbidden because we dont want the user
				// to reauthenticate
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}

			// Checking is successful - user authorized. Here we
			// build a token to pass to the underlying GRPC
			// service with metadata about the user.
			user_info := &api_proto.VelociraptorUser{
				Name: user_record.Name,
			}

			// Must use json encoding because grpc can not handle
			// binary data in metadata.
			serialized, _ := json.Marshal(user_info)
			ctx := context.WithValue(
				r.Context(), constants.GRPC_USER_CONTEXT, string(serialized))

			// Need to call logging after auth so it can access
			// the USER value in the context.
			logger.ServeHTTP(w, r.WithContext(ctx))
		}).AddChild("GetLoggingHandler")
}
