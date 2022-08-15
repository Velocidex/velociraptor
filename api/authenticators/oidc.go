package authenticators

import (
	"context"
	"net/http"
	"strings"

	oidc "github.com/coreos/go-oidc"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

type OidcAuthenticator struct {
	config_obj    *config_proto.Config
	authenticator *config_proto.Authenticator
}

func (self *OidcAuthenticator) IsPasswordLess() bool {
	return true
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
		return "/auth/oidc/" + name + "/login"
	}
	return "/auth/oidc/login"
}

func (self *OidcAuthenticator) LoginURL() string {
	return self.config_obj.GUI.PublicUrl +
		strings.TrimPrefix(self.LoginHandler(), "/")
}

func (self *OidcAuthenticator) CallbackHandler() string {
	name := self.authenticator.OidcName
	if name != "" {
		return "/auth/oidc/" + name + "/callback"
	}
	return "/auth/oidc/callback"
}

func (self *OidcAuthenticator) CallbackURL() string {
	return self.config_obj.GUI.PublicUrl +
		strings.TrimPrefix(self.LoginHandler(), "/")
}

func (self *OidcAuthenticator) AddHandlers(mux *http.ServeMux) error {
	provider, err := oidc.NewProvider(
		context.Background(), self.authenticator.OidcIssuer)
	if err != nil {
		logging.GetLogger(self.config_obj, &logging.GUIComponent).
			Errorf("can not get information from OIDC provider, "+
				"check %v/.well-known/openid-configuration is correct and accessible from the server.",
				self.authenticator.OidcIssuer)
		return err
	}

	mux.Handle(self.LoginHandler(), self.oauthOidcLogin(provider))
	mux.Handle(self.CallbackHandler(), self.oauthOidcCallback(provider))
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
			oauthState = generateStateOauthCookie(w)
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
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		oidcOauthConfig := self.getGenOauthConfig(
			provider.Endpoint(), self.CallbackHandler())
		oauthToken, err := oidcOauthConfig.Exchange(r.Context(), r.FormValue("code"))
		if err != nil {
			logging.GetLogger(self.config_obj, &logging.GUIComponent).
				Error("can not get oauthToken from OIDC provider: %v", err)
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}
		userInfo, err := provider.UserInfo(
			r.Context(), oauth2.StaticTokenSource(oauthToken))
		if err != nil {
			logging.GetLogger(self.config_obj, &logging.GUIComponent).
				Error("can not get UserInfo from OIDC provider: %v", err)
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
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
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		http.SetCookie(w, cookie)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	})
}
