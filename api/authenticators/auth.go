package authenticators

import (
	"errors"
	"fmt"
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
	AuthRedirectTemplate() string
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
	handler, pres := auth_dispatcher[strings.ToLower(auth_config.Type)]
	mu.Unlock()
	if pres {
		return handler(config_obj, auth_config)
	}
	return nil, errors.New("No valid authenticator found")
}

func configRequirePublicUrl(config_obj *config_proto.Config) error {
	if config_obj.GUI.PublicUrl == "" {
		return fmt.Errorf("Authentication type `%s' requires valid public_url parameter",
			config_obj.GUI.Authenticator.Type)
	}
	return nil

}

func init() {
	RegisterAuthenticator("azure", func(config_obj *config_proto.Config,
		auth_config *config_proto.Authenticator) (Authenticator, error) {
		err := configRequirePublicUrl(config_obj)
		if err != nil {
			return nil, err
		}
		return &AzureAuthenticator{
			config_obj:    config_obj,
			authenticator: auth_config,
			base:          getBasePath(config_obj),
			public_url:    getPublicURL(config_obj),
		}, nil
	})

	RegisterAuthenticator("github", func(config_obj *config_proto.Config,
		auth_config *config_proto.Authenticator) (Authenticator, error) {
		err := configRequirePublicUrl(config_obj)
		if err != nil {
			return nil, err
		}
		return &GitHubAuthenticator{
			config_obj:    config_obj,
			authenticator: auth_config,
			base:          getBasePath(config_obj),
			public_url:    getPublicURL(config_obj),
		}, nil
	})

	RegisterAuthenticator("google", func(config_obj *config_proto.Config,
		auth_config *config_proto.Authenticator) (Authenticator, error) {
		err := configRequirePublicUrl(config_obj)
		if err != nil {
			return nil, err
		}
		return &GoogleAuthenticator{
			config_obj:    config_obj,
			authenticator: auth_config,
			base:          getBasePath(config_obj),
			public_url:    getPublicURL(config_obj),
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
			base:       getBasePath(config_obj),
			public_url: getPublicURL(config_obj),
		}, nil
	})

	RegisterAuthenticator("oidc", func(config_obj *config_proto.Config,
		auth_config *config_proto.Authenticator) (Authenticator, error) {
		err := configRequirePublicUrl(config_obj)
		if err != nil {
			return nil, err
		}
		return &OidcAuthenticator{
			config_obj:    config_obj,
			authenticator: auth_config,
			base:          getBasePath(config_obj),
			public_url:    getPublicURL(config_obj),
		}, nil
	})

	RegisterAuthenticator("multi", func(config_obj *config_proto.Config,
		auth_config *config_proto.Authenticator) (Authenticator, error) {
		return NewMultiAuthenticator(config_obj, auth_config)
	})
}

// Ensure base path start and ends with /
func getBasePath(config_obj *config_proto.Config) string {
	bare := strings.TrimSuffix(config_obj.GUI.BasePath, "/")
	bare = strings.TrimPrefix(bare, "/")
	if bare == "" {
		return "/"
	}
	return "/" + bare + "/"
}

// Ensure public URL start and ends with /
func getPublicURL(config_obj *config_proto.Config) string {
	bare := strings.TrimSuffix(config_obj.GUI.PublicUrl, "/")
	return bare + "/"
}
