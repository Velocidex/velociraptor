package authenticators

import (
	"errors"
	"net/http"
	"strings"

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
	if config_obj.GUI == nil ||
		config_obj.GUI.Authenticator == nil ||
		config_obj.Frontend == nil {
		return nil, errors.New("GUI not configured")
	}

	switch strings.ToLower(config_obj.GUI.Authenticator.Type) {
	case "azure":
		return &AzureAuthenticator{}, nil
	case "github":
		return &GitHubAuthenticator{}, nil
	case "google":
		return &GoogleAuthenticator{}, nil
	case "saml":
		return &SamlAuthenticator{}, nil
	case "basic":
		return &BasicAuthenticator{}, nil
	}
	return nil, errors.New("No valid authenticator found")
}
