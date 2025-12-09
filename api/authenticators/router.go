package authenticators

import (
	oidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	api_utils "www.velocidex.com/golang/velociraptor/api/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

// The router gives all the URLs to the relevant endpoints
type OidcRouter interface {
	Name() string
	LoginHandler() string
	CallbackHandler() string
	Scopes() []string
	Issuer() string
	Endpoint() oauth2.Endpoint
	SetEndpoint(oauth2.Endpoint)
	Avatar() string

	LoginURL() string
}

type DefaultOidcRouter struct {
	authenticator *config_proto.Authenticator
	config_obj    *config_proto.Config
	endpoint      oauth2.Endpoint
}

func (self *DefaultOidcRouter) Name() string {
	name := self.authenticator.OidcName
	if name == "" {
		return "Generic OIDC Connector"
	}
	return name
}

func (self *DefaultOidcRouter) LoginHandler() string {
	name := self.authenticator.OidcName
	if name != "" {
		return api_utils.Join("/auth/oidc/", name, "/login")
	}
	return "/auth/oidc/login"
}

func (self *DefaultOidcRouter) LoginURL() string {
	return api_utils.PublicURL(self.config_obj, self.LoginHandler())
}

func (self *DefaultOidcRouter) CallbackHandler() string {
	name := self.authenticator.OidcName
	if name != "" {
		return api_utils.Join("/auth/oidc/", name, "/callback")
	}
	return "/auth/oidc/callback"
}

func (self *DefaultOidcRouter) Scopes() []string {
	return []string{oidc.ScopeOpenID, "email"}
}

func (self *DefaultOidcRouter) Issuer() string {
	return self.authenticator.OidcIssuer
}

func (self *DefaultOidcRouter) Endpoint() oauth2.Endpoint {
	return self.endpoint
}

func (self *DefaultOidcRouter) SetEndpoint(ep oauth2.Endpoint) {
	self.endpoint = ep
}

func (self *DefaultOidcRouter) Avatar() string {
	return self.authenticator.Avatar
}
