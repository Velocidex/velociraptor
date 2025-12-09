package authenticators

import (
	"context"
	"fmt"
	"net/http"
	"time"

	jwt "github.com/golang-jwt/jwt/v4"
	"golang.org/x/oauth2"
	api_utils "www.velocidex.com/golang/velociraptor/api/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	utils "www.velocidex.com/golang/velociraptor/utils"
)

// Abstract oauth and oidc authenticators so we can reuse them in
// different types.

// Transformers allow for stacking of round trippers.
type RoundTripFunc func(req *http.Request) (*http.Response, error)

// A transformer can intercept network communications and change
// them. This is used to add debugging (to see what the oauth server
// is sending) and to mock out the network comms for testing.
type Transformer func(rt RoundTripFunc) RoundTripFunc

// The provider's external methods. For oauth2 we only need two steps
// from the provider: The first is to form the redirect URL to the
// IDP, the second is to process the callback from the IDP and produce
// a JWT which we will store in a session cookie. By abstracting the
// provider we can easily test it.
type ProviderInterface interface {
	GetRedirectURL(options []oauth2.AuthCodeOption, state string) string
	GetJWT(ctx *HTTPClientContext, code string) (*http.Cookie, *Claims, error)
}

// Compose the provider from various interfaces:
// The OidcRouter explains the different endpoints we need to access.

// The ClaimsGetter is used to fetch claims from the server and decode
// the username from them. Velociraptor only cares about how to
// extract the username from the claims.
type Provider struct {
	router        OidcRouter
	claims_getter ClaimsGetter
	config_obj    *config_proto.Config
	oauth_config  *oauth2.Config
	authenticator *config_proto.Authenticator
}

// Where to redirect to the
func (self *Provider) GetRedirectURL(
	options []oauth2.AuthCodeOption, state string) string {
	self.oauth_config.Endpoint = self.router.Endpoint()
	return self.oauth_config.AuthCodeURL(state, options...)
}

// GetJWT contacts the oauth server to fetch claims, creates a user
// object which is encoded in a JWT. The JWT can be verified on each
// subsequent request to ensure the user is logged on.
func (self *Provider) GetJWT(
	ctx *HTTPClientContext, code string) (*http.Cookie, *Claims, error) {

	token, err := self.Exchange(ctx, self.oauth_config, code)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"can not get oauthToken from OIDC provider with code %v: %v",
			code, err)
	}

	claims, err := self.claims_getter.GetClaims(ctx, token)
	if err != nil {
		self.Debug("oauthOidcCallback: Unable to get claims: %v", err)
		return nil, nil, err
	}

	cookie, err := self.getSignedJWTTokenCookie(self.config_obj, claims)
	if err != nil {
		return nil, nil, err
	}

	return cookie, claims, err
}

func (self *Provider) getSignedJWTTokenCookie(
	config_obj *config_proto.Config,
	claims *Claims) (*http.Cookie, error) {
	expiry_min := self.authenticator.DefaultSessionExpiryMin
	if expiry_min == 0 {
		expiry_min = 60 * 24 // 1 Day by default
	}

	// We force expiry in the JWT **as well** as the session
	// cookie. The JWT expiry is most important as the browser can
	// replay session cookies past expiry.
	expiry := utils.GetTime().Now().Add(time.Minute * time.Duration(expiry_min))

	// Enforce the JWT to expire
	claims.Expires = float64(expiry.Unix())

	// Make a JWT and sign it.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	self.Debug("getSignedJWTTokenCookie: Creating JWT with claims: %#v", claims)

	tokenString, err := token.SignedString([]byte(config_obj.Frontend.PrivateKey))
	if err != nil {
		return nil, err
	}

	// Sets the cookie on the browser so it is only valid from the
	// base down.
	return &http.Cookie{
		Name:     "VelociraptorAuth",
		Value:    tokenString,
		Path:     api_utils.GetBaseDirectory(config_obj),
		Secure:   true,
		HttpOnly: true,
		Expires:  expiry,
	}, nil
}

// Gets an oidc token from the server.
func (self *Provider) Exchange(
	ctx context.Context,
	oauth_config *oauth2.Config, code string) (*oauth2.Token, error) {

	oauth_config.Endpoint = self.router.Endpoint()
	return oauth_config.Exchange(ctx, code)
}

// Potentially log debug messages depending on the debug setting.
func (self *Provider) Debug(message string, args ...interface{}) {
	if self.authenticator.OidcDebug {
		logging.GetLogger(self.config_obj, &logging.GUIComponent).
			Debug(message, args...)
	}
}

func NewProvider(
	config_obj *config_proto.Config,
	oauth_config *oauth2.Config,
	authenticator *config_proto.Authenticator,
	router OidcRouter,
	claims_getter ClaimsGetter) (ProviderInterface, error) {
	return &Provider{
		router:        router,
		config_obj:    config_obj,
		oauth_config:  oauth_config,
		authenticator: authenticator,
		claims_getter: claims_getter,
	}, nil
}
