package authenticators

import (
	"context"
	"net/http"
	"strings"

	oidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	"www.velocidex.com/golang/velociraptor/acls"
	api_utils "www.velocidex.com/golang/velociraptor/api/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

type OIDCConnector interface {
	GetGenOauthConfig() (*oauth2.Config, error)
}

type OidcAuthenticator struct {
	config_obj       *config_proto.Config
	authenticator    *config_proto.Authenticator
	base, public_url string
}

func (self *OidcAuthenticator) IsPasswordLess() bool {
	return true
}

func (self *OidcAuthenticator) RequireClientCerts() bool {
	return false
}

func (self *OidcAuthenticator) AuthRedirectTemplate() string {
	return self.authenticator.AuthRedirectTemplate
}

func (self *OidcAuthenticator) Name() string {
	name := self.authenticator.OidcName
	if name == "" {
		return "Generic OIDC Connector"
	}
	return name
}

func (self *OidcAuthenticator) LoginHandler() string {
	name := self.authenticator.OidcName
	if name != "" {
		return api_utils.Join("/auth/oidc/", name, "/login")
	}
	return "/auth/oidc/login"
}

func (self *OidcAuthenticator) LoginURL() string {
	return self.LoginHandler()
}

func (self *OidcAuthenticator) CallbackHandler() string {
	name := self.authenticator.OidcName
	if name != "" {
		return api_utils.Join("/auth/oidc/", name, "/callback")
	}
	return "/auth/oidc/callback"
}

func (self *OidcAuthenticator) CallbackURL() string {
	return self.LoginHandler()
}

func (self *OidcAuthenticator) GetProvider() (*oidc.Provider, error) {
	ctx, err := ClientContext(context.Background(), self.config_obj)
	if err != nil {
		return nil, err
	}
	return oidc.NewProvider(ctx, self.authenticator.OidcIssuer)
}

func (self *OidcAuthenticator) AddHandlers(mux *api_utils.ServeMux) error {
	provider, err := self.GetProvider()
	if err != nil {
		logging.GetLogger(self.config_obj, &logging.GUIComponent).
			Errorf("can not get information from OIDC provider, "+
				"check %v/.well-known/openid-configuration is correct and accessible from the server.",
				self.authenticator.OidcIssuer)
		return err
	}

	mux.Handle(api_utils.GetBasePath(self.config_obj, self.LoginHandler()),
		IpFilter(self.config_obj, self.oauthOidcLogin(provider)))
	mux.Handle(api_utils.GetBasePath(self.config_obj, self.CallbackHandler()),
		IpFilter(self.config_obj, self.oauthOidcCallback(provider)))
	return nil
}

func (self *OidcAuthenticator) AddLogoff(mux *api_utils.ServeMux) error {
	installLogoff(self.config_obj, mux)
	return nil
}

func (self *OidcAuthenticator) AuthenticateUserHandler(
	parent http.Handler,
	permission acls.ACL_PERMISSION,
) http.Handler {
	return authenticateUserHandle(
		self.config_obj, permission,
		func(w http.ResponseWriter, r *http.Request, err error, username string) {
			reject_with_username(self.config_obj, w, r, err, username,
				self.LoginURL(), self.Name())
		},
		parent)
}

func (self *OidcAuthenticator) GetGenOauthConfig() (*oauth2.Config, error) {

	callback := self.CallbackHandler()

	var scope []string
	switch strings.ToLower(self.authenticator.Type) {
	case "oidc", "oidc-cognito":
		scope = []string{oidc.ScopeOpenID, "email"}
	}

	return &oauth2.Config{
		RedirectURL:  api_utils.GetPublicURL(self.config_obj, callback),
		ClientID:     self.authenticator.OauthClientId,
		ClientSecret: self.authenticator.OauthClientSecret,
		Scopes:       scope,
	}, nil
}

func (self *OidcAuthenticator) oauthOidcLogin(
	provider *oidc.Provider) http.Handler {

	return api_utils.HandlerFunc(nil,
		func(w http.ResponseWriter, r *http.Request) {
			oidcOauthConfig, err := self.GetGenOauthConfig()
			if err != nil {
				logging.GetLogger(self.config_obj, &logging.GUIComponent).
					Error("GetGenOauthConfig: %v", err)
				http.Error(w, "rejected", http.StatusUnauthorized)
			}
			oidcOauthConfig.Endpoint = provider.Endpoint()

			utils.Debug(oidcOauthConfig)

			// Create oauthState cookie
			oauthState, err := r.Cookie("oauthstate")
			if err != nil {
				oauthState = generateStateOauthCookie(self.config_obj, w)
			}

			url := oidcOauthConfig.AuthCodeURL(oauthState.Value)

			// Needed for Okta to specify `prompt: login` to avoid consent
			// auth on each login.
			if self.authenticator.OidcAuthUrlParams != nil {
				for k, v := range self.authenticator.OidcAuthUrlParams {
					oauth2.SetAuthURLParam(k, v)
				}
			}

			http.Redirect(w, r, url, http.StatusFound)
		})
}

func (self *OidcAuthenticator) oauthOidcCallback(
	provider *oidc.Provider) http.Handler {
	return api_utils.HandlerFunc(nil,
		func(w http.ResponseWriter, r *http.Request) {
			// Read oauthState from Cookie
			oauthState, _ := r.Cookie("oauthstate")
			if oauthState == nil || r.FormValue("state") != oauthState.Value {
				logging.GetLogger(self.config_obj, &logging.GUIComponent).
					Error("invalid oauth state of OIDC")
				http.Redirect(w, r, api_utils.Homepage(self.config_obj),
					http.StatusTemporaryRedirect)
				return
			}

			oidcOauthConfig, err := self.GetGenOauthConfig()
			if err != nil {
				logging.GetLogger(self.config_obj, &logging.GUIComponent).
					Error("GetGenOauthConfig: %v", err)
				http.Error(w, "rejected", http.StatusUnauthorized)
			}
			oidcOauthConfig.Endpoint = provider.Endpoint()

			ctx, err := ClientContext(r.Context(), self.config_obj)
			if err != nil {
				logging.GetLogger(self.config_obj, &logging.GUIComponent).
					Error("invalid client context of OIDC")
				http.Redirect(w, r, api_utils.Homepage(self.config_obj),
					http.StatusTemporaryRedirect)
				return
			}
			oauthToken, err := oidcOauthConfig.Exchange(ctx, r.FormValue("code"))
			if err != nil {
				logging.GetLogger(self.config_obj, &logging.GUIComponent).
					Error("can not get oauthToken from OIDC provider: %v", err)
				http.Redirect(w, r, api_utils.Homepage(self.config_obj),
					http.StatusTemporaryRedirect)
				return
			}
			userInfo, err := provider.UserInfo(
				ctx, oauth2.StaticTokenSource(oauthToken))
			if err != nil {
				logging.GetLogger(self.config_obj, &logging.GUIComponent).
					Error("can not get UserInfo from OIDC provider: %v", err)
				http.Redirect(w, r, api_utils.Homepage(self.config_obj),
					http.StatusTemporaryRedirect)
				return
			}

			cookie, err := getSignedJWTTokenCookie(
				self.config_obj, self.authenticator,
				&Claims{
					Username: userInfo.Email,
				})
			if err != nil {
				logging.GetLogger(self.config_obj, &logging.GUIComponent).
					WithFields(logrus.Fields{
						"err": err.Error(),
					}).Error("can not get a signed tokenString")
				http.Redirect(w, r, api_utils.Homepage(self.config_obj),
					http.StatusTemporaryRedirect)
				return
			}

			http.SetCookie(w, cookie)
			http.Redirect(w, r, api_utils.Homepage(self.config_obj),
				http.StatusTemporaryRedirect)
		})
}
