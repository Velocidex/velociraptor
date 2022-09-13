package authenticators

import (
	"fmt"
	"net/http"

	"github.com/sirupsen/logrus"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/gui/velociraptor"
	"www.velocidex.com/golang/velociraptor/logging"
)

type MultiAuthenticator struct {
	delegates     []Authenticator
	config_obj    *config_proto.Config
	delegate_info []velociraptor.AuthenticatorInfo
}

func (self *MultiAuthenticator) AddHandlers(mux *http.ServeMux) error {
	for _, delegate := range self.delegates {
		err := delegate.AddHandlers(mux)
		if err != nil {
			return err
		}
	}
	return nil
}

func (self *MultiAuthenticator) AddLogoff(mux *http.ServeMux) error {
	installLogoff(self.config_obj, mux)
	return nil
}

func (self *MultiAuthenticator) reject_with_username(
	w http.ResponseWriter, r *http.Request, err error, username string) {
	logger := logging.GetLogger(self.config_obj, &logging.Audit)

	// Log into the audit log.
	logger.WithFields(logrus.Fields{
		"user":   username,
		"remote": r.RemoteAddr,
		"method": r.Method,
		"err":    err.Error(),
	}).Error("User rejected by GUI")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)

	renderRejectionMessage(self.config_obj, w, username, self.delegate_info)
}

func (self *MultiAuthenticator) AuthenticateUserHandler(
	parent http.Handler) http.Handler {

	return authenticateUserHandle(
		self.config_obj,
		func(w http.ResponseWriter, r *http.Request, err error, username string) {
			self.reject_with_username(w, r, err, username)
		},
		parent)
}

func (self *MultiAuthenticator) IsPasswordLess() bool {
	return true
}

func NewMultiAuthenticator(
	config_obj *config_proto.Config,
	auth_config *config_proto.Authenticator) (Authenticator, error) {
	result := &MultiAuthenticator{
		config_obj: config_obj,
	}
	for _, authenticator_config := range auth_config.SubAuthenticators {
		auth, err := getAuthenticatorByType(config_obj, authenticator_config)
		if err != nil {
			return nil, err
		}

		// Only accept supported sub types.
		switch t := auth.(type) {
		case *GitHubAuthenticator:
			result.delegate_info = append(result.delegate_info,
				velociraptor.AuthenticatorInfo{
					LoginURL:     t.LoginURL(),
					ProviderName: "Github",
				})

		case *GoogleAuthenticator:
			result.delegate_info = append(result.delegate_info,
				velociraptor.AuthenticatorInfo{
					LoginURL:     t.LoginURL(),
					ProviderName: `Google`,
				})

		case *AzureAuthenticator:
			result.delegate_info = append(result.delegate_info,
				velociraptor.AuthenticatorInfo{
					LoginURL:     t.LoginURL(),
					ProviderName: `Microsoft`,
				})

		case *OidcAuthenticator:
			result.delegate_info = append(result.delegate_info,
				velociraptor.AuthenticatorInfo{
					LoginURL:       t.LoginURL(),
					ProviderName:   t.authenticator.OidcName,
					ProviderAvatar: t.authenticator.Avatar,
				})

		case *OidcAuthenticatorCognito:
			result.delegate_info = append(result.delegate_info,
				velociraptor.AuthenticatorInfo{
					LoginURL:       t.LoginURL(),
					ProviderName:   t.authenticator.OidcName,
					ProviderAvatar: t.authenticator.Avatar,
				})

		default:
			return nil, fmt.Errorf("MultiAuthenticator does not support %v as a child authenticator",
				authenticator_config.Type)
		}

		result.delegates = append(result.delegates, auth)
	}

	return result, nil
}
