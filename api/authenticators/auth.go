package authenticators

import (
	"errors"
	"net/http"
	"strings"
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

var (
	mu sync.Mutex

	auth_dispatcher = make(map[string]func(
		config_obj *config_proto.Config,
		auth_config *config_proto.Authenticator) (Authenticator, error))
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

func RegisterAuthenticator(name string,
	handler func(config_obj *config_proto.Config,
		auth_config *config_proto.Authenticator) (Authenticator, error)) {
	mu.Lock()
	defer mu.Unlock()

	auth_dispatcher[strings.ToLower(name)] = handler
}

func getAuthenticatorByType(
	config_obj *config_proto.Config,
	auth_config *config_proto.Authenticator) (Authenticator, error) {

	mu.Lock()
	defer mu.Unlock()

	handler, pres := auth_dispatcher[strings.ToLower(auth_config.Type)]
	if pres {
		return handler(config_obj, auth_config)
	}
	return nil, errors.New("No valid authenticator found")
}

func init() {
	RegisterAuthenticator("azure", func(config_obj *config_proto.Config,
		auth_config *config_proto.Authenticator) (Authenticator, error) {
		return &AzureAuthenticator{
			config_obj:    config_obj,
			authenticator: auth_config,
		}, nil
	})

	RegisterAuthenticator("github", func(config_obj *config_proto.Config,
		auth_config *config_proto.Authenticator) (Authenticator, error) {
		return &GitHubAuthenticator{
			config_obj:    config_obj,
			authenticator: auth_config,
		}, nil
	})

	RegisterAuthenticator("google", func(config_obj *config_proto.Config,
		auth_config *config_proto.Authenticator) (Authenticator, error) {
		return &GoogleAuthenticator{
			config_obj:    config_obj,
			authenticator: auth_config,
		}, nil
	})

	RegisterAuthenticator("saml", func(config_obj *config_proto.Config,
		auth_config *config_proto.Authenticator) (Authenticator, error) {
		return NewSamlAuthenticator(config_obj, auth_config)
	})

	RegisterAuthenticator("basic", func(config_obj *config_proto.Config,
		auth_config *config_proto.Authenticator) (Authenticator, error) {
		return &BasicAuthenticator{
			config_obj: config_obj,
		}, nil
	})

	RegisterAuthenticator("oidc", func(config_obj *config_proto.Config,
		auth_config *config_proto.Authenticator) (Authenticator, error) {
		return &OidcAuthenticator{
			config_obj:    config_obj,
			authenticator: auth_config,
		}, nil
	})

	RegisterAuthenticator("multi", func(config_obj *config_proto.Config,
		auth_config *config_proto.Authenticator) (Authenticator, error) {
		return NewMultiAuthenticator(config_obj, auth_config)
	})
}
