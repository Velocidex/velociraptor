package authenticators

import (
	"errors"
	"net/http"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

// All SSO Authenticators implement this interface.
type Authenticator interface {
	AddHandlers(config_obj *config_proto.Config, mux *http.ServeMux) error
	AuthenticateUserHandler(
		config_obj *config_proto.Config,
		parent http.Handler) http.Handler

	IsPasswordLess() bool
}

func NewAuthenticator(config_obj *config_proto.Config) (Authenticator, error) {
	if config_obj.GUI == nil || config_obj.GUI.Authenticator == nil {
		return nil, errors.New("GUI not configured")
	}

	switch config_obj.GUI.Authenticator.Type {
	case "Azure":
		return &AzureAuthenticator{}, nil
	case "Github":
		return &GitHubAuthenticator{}, nil
	case "Google":
		return &GoogleAuthenticator{}, nil
	case "SAML":
		return &SamlAuthenticator{}, nil
	case "Basic":
		return &BasicAuthenticator{}, nil
	}
	return nil, errors.New("No valid authenticator found")
}
