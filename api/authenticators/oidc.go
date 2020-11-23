package authenticators

import (
	"context"
	"net/http"
	"strings"
	"time"

	oidc "github.com/coreos/go-oidc"
	jwt "github.com/dgrijalva/jwt-go"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

const (
	oidcLoginURI    = "/auth/oidc/login"
	oidcCallbackURI = "/auth/oidc/callback"
)

type OidcAuthenticator struct{}

func (self *OidcAuthenticator) IsPasswordLess() bool {
	return true
}

func (*OidcAuthenticator) AddHandlers(config_obj *config_proto.Config, mux *http.ServeMux) error {
	provider, err := oidc.NewProvider(context.Background(),
		config_obj.GUI.Authenticator.OidcIssuer)
	if err != nil {
		logging.GetLogger(config_obj, &logging.GUIComponent).
			Errorf("can not get information from OIDC provider, "+
				"check %v/.well-known/openid-configuration is correct and accessible from the server.",
				config_obj.GUI.Authenticator.OidcIssuer)
		return err
	}

	mux.Handle(oidcLoginURI, oauthOidcLogin(config_obj, provider))
	mux.Handle(oidcCallbackURI, oauthOidcCallback(config_obj, provider))

	installLogoff(config_obj, mux)
	return nil
}

func (*OidcAuthenticator) AuthenticateUserHandler(
	config_obj *config_proto.Config,
	parent http.Handler) http.Handler {
	return authenticateUserHandle(
		config_obj, parent, oidcLoginURI, "OIDC")
}

func getGenOauthConfig(
	config_obj *config_proto.Config,
	endpoint oauth2.Endpoint,
	callback string) *oauth2.Config {

	var scope []string
	switch strings.ToLower(config_obj.GUI.Authenticator.Type) {
	case "oidc":
		scope = []string{oidc.ScopeOpenID, "email"}
	}

	return &oauth2.Config{
		RedirectURL:  config_obj.GUI.PublicUrl + callback[1:],
		ClientID:     config_obj.GUI.Authenticator.OauthClientId,
		ClientSecret: config_obj.GUI.Authenticator.OauthClientSecret,
		Scopes:       scope,
		Endpoint:     endpoint,
	}
}

func oauthOidcLogin(config_obj *config_proto.Config, provider *oidc.Provider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		oidcOauthConfig := getGenOauthConfig(config_obj, provider.Endpoint(), oidcCallbackURI)

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

func oauthOidcCallback(config_obj *config_proto.Config, provider *oidc.Provider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read oauthState from Cookie
		oauthState, _ := r.Cookie("oauthstate")
		if oauthState == nil || r.FormValue("state") != oauthState.Value {
			logging.GetLogger(config_obj, &logging.GUIComponent).
				Error("invalid oauth state of OIDC")
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		oidcOauthConfig := getGenOauthConfig(config_obj, provider.Endpoint(), oidcCallbackURI)
		oauthToken, err := oidcOauthConfig.Exchange(r.Context(), r.FormValue("code"))
		if err != nil {
			logging.GetLogger(config_obj, &logging.GUIComponent).
				Error("can not get oauthToken from OIDC provider: %v", err)
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}
		userInfo, err := provider.UserInfo(r.Context(), oauth2.StaticTokenSource(oauthToken))
		if err != nil {
			logging.GetLogger(config_obj, &logging.GUIComponent).
				Error("can not get UserInfo from OIDC provider")
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"user":    userInfo.Email,
			"expires": float64(time.Now().AddDate(0, 0, 1).Unix()),
		})

		tokenString, err := token.SignedString(
			[]byte(config_obj.Frontend.PrivateKey))
		if err != nil {
			logging.GetLogger(config_obj, &logging.GUIComponent).
				WithFields(logrus.Fields{
					"err": err,
				}).Error("can not get a signed tokenString")
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		cookie := &http.Cookie{
			Name:    "VelociraptorAuth",
			Value:   tokenString,
			Path:    "/",
			Expires: time.Now().AddDate(0, 0, 1),
		}
		http.SetCookie(w, cookie)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	})
}
