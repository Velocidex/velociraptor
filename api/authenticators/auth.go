package authenticators

import (
	"errors"
	"net/http"
	"strings"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

// All SSO Authenticators implement this interface.
type Authenticator interface {
	AddHandlers(mux *http.ServeMux) error
	AddLogoff(mux *http.ServeMux) error
	AuthenticateUserHandler(parent http.Handler) http.Handler

	IsPasswordLess() bool
}

func NewAuthenticator(config_obj *config_proto.Config) (Authenticator, error) {
	if config_obj.GUI == nil ||
		config_obj.GUI.Authenticator == nil ||
		config_obj.Frontend == nil {
		return nil, errors.New("GUI not configured")
	}

	return getAuthenticatorByType(config_obj, config_obj.GUI.Authenticator)
}

func getAuthenticatorByType(
	config_obj *config_proto.Config,
	auth_config *config_proto.Authenticator) (Authenticator, error) {
	auth_type := strings.ToLower(auth_config.Type)
	switch auth_type {
	case "azure":
		return &AzureAuthenticator{
			config_obj:    config_obj,
			authenticator: auth_config,
		}, nil
	case "github":
		return &GitHubAuthenticator{
			config_obj:    config_obj,
			authenticator: auth_config,
		}, nil
	case "google":
		return &GoogleAuthenticator{
			config_obj:    config_obj,
			authenticator: auth_config,
		}, nil
	case "saml":
		return NewSamlAuthenticator(config_obj, auth_config)

	case "basic":
		return &BasicAuthenticator{
			config_obj: config_obj,
		}, nil
	case "oidc":
		return &OidcAuthenticator{
			config_obj:    config_obj,
			authenticator: auth_config,
		}, nil

	case "multi":
		return NewMultiAuthenticator(config_obj, auth_config)
	}
	return nil, errors.New("No valid authenticator found")
}
