package authenticators

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"www.velocidex.com/golang/velociraptor/acls"
	utils "www.velocidex.com/golang/velociraptor/api/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

var (
	mu sync.Mutex

	// Factory dispatcher
	auth_dispatcher = make(map[string]func(
		ctx *HTTPClientContext,
		config_obj *config_proto.Config,
		auth_config *config_proto.Authenticator) (Authenticator, error))

	auth_cache Authenticator
)

func ResetAuthCache() {
	mu.Lock()
	defer mu.Unlock()
	auth_cache = nil
}

// All SSO Authenticators implement this interface.
type Authenticator interface {
	AddHandlers(mux *utils.ServeMux) error
	AddLogoff(mux *utils.ServeMux) error

	// Make sure the user is authenticated and has the required
	// permission access to the requested org. (usually this is
	// acls.READ_RESULTS)
	AuthenticateUserHandler(parent http.Handler,
		permission acls.ACL_PERMISSION) http.Handler

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

	mu.Lock()
	cached := auth_cache
	mu.Unlock()

	if cached != nil {
		return cached, nil
	}

	ctx, err := ClientContext(context.Background(), config_obj,
		DefaultTransforms(config_obj, config_obj.GUI.Authenticator))
	if err != nil {
		return nil, err
	}

	new_auth, err := getAuthenticatorByType(
		ctx, config_obj, config_obj.GUI.Authenticator)
	if err == nil {
		mu.Lock()
		auth_cache = new_auth
		mu.Unlock()
	}

	return new_auth, err
}

func RegisterAuthenticator(name string,
	handler func(
		ctx *HTTPClientContext,
		config_obj *config_proto.Config,
		auth_config *config_proto.Authenticator) (Authenticator, error)) {
	mu.Lock()
	defer mu.Unlock()

	auth_dispatcher[strings.ToLower(name)] = handler
}

func getAuthenticatorByType(
	ctx *HTTPClientContext,
	config_obj *config_proto.Config,
	auth_config *config_proto.Authenticator) (Authenticator, error) {

	mu.Lock()
	key := strings.ToLower(auth_config.Type)
	handler, pres := auth_dispatcher[key]
	mu.Unlock()
	if pres {
		// Make sure the dispatcher lock is unlocked during call to
		// handler - the multi authenticator needs to access the
		// other types.
		return handler(ctx, config_obj, auth_config)
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
	RegisterAuthenticator("azure", func(
		ctx *HTTPClientContext,
		config_obj *config_proto.Config,
		auth_config *config_proto.Authenticator) (Authenticator, error) {
		err := configRequirePublicUrl(config_obj)
		if err != nil {
			return nil, err
		}
		router := &AzureOidcRouter{
			config_obj:    config_obj,
			authenticator: auth_config,
		}
		claims_getter := &AzureClaimsGetter{
			config_obj: config_obj,
			router:     router,
		}
		return NewOidcAuthenticator(
			config_obj, auth_config, router, claims_getter), nil
	})

	RegisterAuthenticator("github", func(
		ctx *HTTPClientContext,
		config_obj *config_proto.Config,
		auth_config *config_proto.Authenticator) (Authenticator, error) {
		err := configRequirePublicUrl(config_obj)
		if err != nil {
			return nil, err
		}

		router := &GithubOidcRouter{
			config_obj: config_obj,
		}
		claims_getter := &GithubClaimsGetter{
			config_obj: config_obj,
		}
		return NewOidcAuthenticator(
			config_obj, auth_config, router, claims_getter), nil
	})

	// This is now basically an alias for a generic OIDC connector
	// since Google is pretty good about following the standards.
	RegisterAuthenticator("google", func(
		ctx *HTTPClientContext,
		config_obj *config_proto.Config,
		auth_config *config_proto.Authenticator) (Authenticator, error) {
		err := configRequirePublicUrl(config_obj)
		if err != nil {
			return nil, err
		}

		router := &GoogleOidcRouter{
			config_obj: config_obj,
		}

		claims_getter, err := NewOidcClaimsGetter(
			ctx, config_obj, auth_config, router)
		if err != nil {
			return nil, err
		}

		return NewOidcAuthenticator(
			config_obj, auth_config, router, claims_getter), nil
	})

	RegisterAuthenticator("saml", func(
		ctx *HTTPClientContext,
		config_obj *config_proto.Config,
		auth_config *config_proto.Authenticator) (Authenticator, error) {
		return NewSamlAuthenticator(config_obj, auth_config)
	})

	RegisterAuthenticator("basic", func(
		ctx *HTTPClientContext,
		config_obj *config_proto.Config,
		auth_config *config_proto.Authenticator) (Authenticator, error) {
		return &BasicAuthenticator{
			config_obj: config_obj,
		}, nil
	})

	RegisterAuthenticator("certs", func(
		ctx *HTTPClientContext,
		config_obj *config_proto.Config,
		auth_config *config_proto.Authenticator) (Authenticator, error) {
		if config_obj.GUI == nil || config_obj.GUI.UsePlainHttp {
			return nil, errors.New("'Certs' authenticator must use TLS!")
		}

		result := &CertAuthenticator{
			config_obj:    config_obj,
			x509_roots:    x509.NewCertPool(),
			default_roles: auth_config.DefaultRolesForUnknownUser,
		}
		if config_obj.Client != nil {
			result.x509_roots.AppendCertsFromPEM([]byte(
				config_obj.Client.CaCertificate))
		}

		return result, nil
	})

	RegisterAuthenticator("oidc", func(
		ctx *HTTPClientContext,
		config_obj *config_proto.Config,
		auth_config *config_proto.Authenticator) (Authenticator, error) {
		err := configRequirePublicUrl(config_obj)
		if err != nil {
			return nil, err
		}

		router := &DefaultOidcRouter{
			authenticator: auth_config,
			config_obj:    config_obj,
		}

		claims_getter, err := NewOidcClaimsGetter(
			ctx, config_obj, auth_config, router)
		if err != nil {
			return nil, err
		}

		return NewOidcAuthenticator(
			config_obj, auth_config, router, claims_getter), nil
	})

	RegisterAuthenticator("multi", func(
		ctx *HTTPClientContext,
		config_obj *config_proto.Config,
		auth_config *config_proto.Authenticator) (Authenticator, error) {
		return NewMultiAuthenticator(ctx, config_obj, auth_config)
	})
}
