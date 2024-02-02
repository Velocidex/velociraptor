package authenticators

import (
	"context"
	"net/http"
	"strings"

	oidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	utils "www.velocidex.com/golang/velociraptor/api/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

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
		return utils.Join(self.base, "/auth/oidc/", name, "/login")
	}
	return utils.Join(self.base, "/auth/oidc/login")
}

func (self *OidcAuthenticator) LoginURL() string {
	return utils.Join(self.public_url, self.LoginHandler())
}

func (self *OidcAuthenticator) CallbackHandler() string {
	name := self.authenticator.OidcName
	if name != "" {
		return utils.Join(self.base, "/auth/oidc/", name, "/callback")
	}
	return utils.Join(self.base, "/auth/oidc/callback")
}

func (self *OidcAuthenticator) CallbackURL() string {
	return utils.Join(self.public_url, self.LoginHandler())
}

func (self *OidcAuthenticator) AddHandlers(mux *http.ServeMux) error {
	ctx, err := ClientContext(context.Background(), self.config_obj)
	if err != nil {
		return err
	}
	provider, err := oidc.NewProvider(ctx, self.authenticator.OidcIssuer)
	if err != nil {
		logging.GetLogger(self.config_obj, &logging.GUIComponent).
			Errorf("can not get information from OIDC provider, "+
				"check %v/.well-known/openid-configuration is correct and accessible from the server.",
				self.authenticator.OidcIssuer)
		return err
	}

	mux.Handle(self.LoginHandler(),
		IpFilter(self.config_obj, self.oauthOidcLogin(provider)))
	mux.Handle(self.CallbackHandler(),
		IpFilter(self.config_obj, self.oauthOidcCallback(provider)))
	return nil
}

func (self *OidcAuthenticator) AddLogoff(mux *http.ServeMux) error {
	installLogoff(self.config_obj, mux)
	return nil
}

func (self *OidcAuthenticator) AuthenticateUserHandler(
	parent http.Handler) http.Handler {
	return authenticateUserHandle(
		self.config_obj,
		func(w http.ResponseWriter, r *http.Request, err error, username string) {
			reject_with_username(self.config_obj, w, r, err, username,
				self.LoginURL(), self.Name())
		},
		parent)
}

func (self *OidcAuthenticator) getGenOauthConfig(
	endpoint oauth2.Endpoint, callback string) *oauth2.Config {

	var scope []string
	switch strings.ToLower(self.authenticator.Type) {
	case "oidc", "oidc-cognito":
		scope = []string{oidc.ScopeOpenID, "email"}
	}

	return &oauth2.Config{
		RedirectURL:  self.config_obj.GUI.PublicUrl + callback[1:],
		ClientID:     self.authenticator.OauthClientId,
		ClientSecret: self.authenticator.OauthClientSecret,
		Scopes:       scope,
		Endpoint:     endpoint,
	}
}

func (self *OidcAuthenticator) oauthOidcLogin(
	provider *oidc.Provider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		oidcOauthConfig := self.getGenOauthConfig(
			provider.Endpoint(), self.CallbackHandler())

		// Create oauthState cookie
		oauthState, err := r.Cookie("oauthstate")
		if err != nil {
			oauthState = generateStateOauthCookie(self.config_obj, w)
		}

		url := oidcOauthConfig.AuthCodeURL(oauthState.Value,
			oauth2.SetAuthURLParam("prompt", "login"))
		http.Redirect(w, r, url, http.StatusFound)
	})
}

func (self *OidcAuthenticator) oauthOidcCallback(
	provider *oidc.Provider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read oauthState from Cookie
		oauthState, _ := r.Cookie("oauthstate")
		if oauthState == nil || r.FormValue("state") != oauthState.Value {
			logging.GetLogger(self.config_obj, &logging.GUIComponent).
				Error("invalid oauth state of OIDC")
			http.Redirect(w, r, utils.Homepage(self.config_obj),
				http.StatusTemporaryRedirect)
			return
		}

		oidcOauthConfig := self.getGenOauthConfig(
			provider.Endpoint(), self.CallbackHandler())

		ctx, err := ClientContext(r.Context(), self.config_obj)
		if err != nil {
			logging.GetLogger(self.config_obj, &logging.GUIComponent).
				Error("invalid client context of OIDC")
			http.Redirect(w, r, utils.Homepage(self.config_obj),
				http.StatusTemporaryRedirect)
			return
		}
		oauthToken, err := oidcOauthConfig.Exchange(ctx, r.FormValue("code"))
		if err != nil {
			logging.GetLogger(self.config_obj, &logging.GUIComponent).
				Error("can not get oauthToken from OIDC provider: %v", err)
			http.Redirect(w, r, utils.Homepage(self.config_obj),
				http.StatusTemporaryRedirect)
			return
		}
		userInfo, err := provider.UserInfo(
			ctx, oauth2.StaticTokenSource(oauthToken))
		if err != nil {
			logging.GetLogger(self.config_obj, &logging.GUIComponent).
				Error("can not get UserInfo from OIDC provider: %v", err)
			http.Redirect(w, r, utils.Homepage(self.config_obj),
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
			http.Redirect(w, r, utils.Homepage(self.config_obj),
				http.StatusTemporaryRedirect)
			return
		}

		http.SetCookie(w, cookie)
		http.Redirect(w, r, utils.Homepage(self.config_obj),
			http.StatusTemporaryRedirect)
	})
}
