package authenticators

import (
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	utils "www.velocidex.com/golang/velociraptor/api/utils"
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

	// Make sure the user is authenticated and has at least read
	// access to the requested org.
	AuthenticateUserHandler(parent http.Handler) http.Handler

	IsPasswordLess() bool
	RequireClientCerts() bool
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
			base:          utils.GetBasePath(config_obj),
			public_url:    utils.GetPublicURL(config_obj),
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
			base:          utils.GetBasePath(config_obj),
			public_url:    utils.GetPublicURL(config_obj),
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
			base:          utils.GetBasePath(config_obj),
			public_url:    utils.GetPublicURL(config_obj),
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
			base:       utils.GetBasePath(config_obj),
			public_url: utils.GetPublicURL(config_obj),
		}, nil
	})

	RegisterAuthenticator("certs", func(config_obj *config_proto.Config,
		auth_config *config_proto.Authenticator) (Authenticator, error) {
		if config_obj.GUI == nil || config_obj.GUI.UsePlainHttp {
			return nil, errors.New("'Certs' authenticator must use TLS!")
		}

		result := &CertAuthenticator{
			config_obj:    config_obj,
			base:          utils.GetBasePath(config_obj),
			public_url:    utils.GetPublicURL(config_obj),
			x509_roots:    x509.NewCertPool(),
			default_roles: auth_config.DefaultRolesForUnknownUser,
		}
		if config_obj.Client != nil {
			result.x509_roots.AppendCertsFromPEM([]byte(
				config_obj.Client.CaCertificate))
		}

		return result, nil
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
			base:          utils.GetBasePath(config_obj),
			public_url:    utils.GetPublicURL(config_obj),
		}, nil
	})

	RegisterAuthenticator("multi", func(config_obj *config_proto.Config,
		auth_config *config_proto.Authenticator) (Authenticator, error) {
		return NewMultiAuthenticator(config_obj, auth_config)
	})
}
