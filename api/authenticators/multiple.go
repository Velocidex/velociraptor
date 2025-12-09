package authenticators

import (
	"fmt"
	"net/http"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	api_utils "www.velocidex.com/golang/velociraptor/api/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/gui/velociraptor"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
)

type MultiAuthenticator struct {
	delegates     []Authenticator
	config_obj    *config_proto.Config
	delegate_info []velociraptor.AuthenticatorInfo
}

func (self *MultiAuthenticator) Delegates() []Authenticator {
	return self.delegates
}

func (self *MultiAuthenticator) AddHandlers(mux *api_utils.ServeMux) error {
	for _, delegate := range self.delegates {
		err := delegate.AddHandlers(mux)
		if err != nil {
			return err
		}
	}
	return nil
}

func (self *MultiAuthenticator) AddLogoff(mux *api_utils.ServeMux) error {
	installLogoff(self.config_obj, mux)
	return nil
}

func (self *MultiAuthenticator) reject_with_username(
	w http.ResponseWriter, r *http.Request, err error, username string) {

	// Log into the audit log.
	if username != "" {
		err := services.LogAudit(r.Context(),
			self.config_obj, username, "User rejected by GUI",
			ordereddict.NewDict().
				Set("remote", r.RemoteAddr).
				Set("method", r.Method).
				Set("err", err.Error()))
		if err != nil {
			logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
			logger.Error("<red>MultiAuthenticator</> reject_with_username %v %v",
				username, r.RemoteAddr)
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)

	renderRejectionMessage(
		self.config_obj, r, w, err,
		username, self.delegate_info)
}

func (self *MultiAuthenticator) AuthenticateUserHandler(
	parent http.Handler,
	permission acls.ACL_PERMISSION,
) http.Handler {

	return authenticateUserHandle(
		self.config_obj, permission,
		func(w http.ResponseWriter, r *http.Request, err error, username string) {
			self.reject_with_username(w, r, err, username)
		},
		parent)
}

func (self *MultiAuthenticator) IsPasswordLess() bool {
	return true
}

func (self *MultiAuthenticator) RequireClientCerts() bool {
	return false
}

func (self *MultiAuthenticator) AuthRedirectTemplate() string {
	return ""
}

func NewMultiAuthenticator(
	ctx *HTTPClientContext,
	config_obj *config_proto.Config,
	auth_config *config_proto.Authenticator) (Authenticator, error) {
	result := &MultiAuthenticator{
		config_obj: config_obj,
	}
	for _, authenticator_config := range auth_config.SubAuthenticators {
		auth, err := getAuthenticatorByType(
			ctx, config_obj, authenticator_config)
		if err != nil {
			return nil, err
		}

		// Only accept supported sub types.
		switch t := auth.(type) {
		case *OidcAuthenticator:
			result.delegate_info = append(result.delegate_info,
				velociraptor.AuthenticatorInfo{
					LoginURL:       t.router.LoginURL(),
					ProviderName:   t.router.Name(),
					ProviderAvatar: t.router.Avatar(),
				})

		default:
			return nil, fmt.Errorf("MultiAuthenticator does not support %v as a child authenticator",
				authenticator_config.Type)
		}

		result.delegates = append(result.delegates, auth)
	}

	return result, nil
}
