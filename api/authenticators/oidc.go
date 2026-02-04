package authenticators

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"time"

	"github.com/Velocidex/ordereddict"
	"golang.org/x/oauth2"
	"www.velocidex.com/golang/velociraptor/acls"
	api_utils "www.velocidex.com/golang/velociraptor/api/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	utils "www.velocidex.com/golang/velociraptor/utils"
)

type OidcAuthenticator struct {
	router        OidcRouter
	claims_getter ClaimsGetter
	config_obj    *config_proto.Config
	authenticator *config_proto.Authenticator
}

func (self *OidcAuthenticator) Name() string {
	return self.router.Name()
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

func (self *OidcAuthenticator) Provider() (ProviderInterface, error) {
	oauth_config, err := self.GetGenOauthConfig()
	if err != nil {
		return nil, err
	}

	return NewProvider(
		self.config_obj, oauth_config, self.authenticator, self.router,
		self.claims_getter)
}

func (self *OidcAuthenticator) AddHandlers(mux *api_utils.ServeMux) error {
	provider, err := self.Provider()
	if err != nil {
		self.Error("can not get information from OIDC provider, "+
			"check %v/.well-known/openid-configuration is correct and accessible from the server.",
			self.authenticator.OidcIssuer)
		return err
	}

	mux.Handle(api_utils.GetBasePath(
		self.config_obj, self.router.LoginHandler()),
		IpFilter(self.config_obj, self.oauthOidcLogin(provider)))
	mux.Handle(api_utils.GetBasePath(
		self.config_obj, self.router.CallbackHandler()),
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
				self.router.LoginHandler(), self.router.Name(), self.router.Avatar())
		},
		parent)
}

func (self *OidcAuthenticator) GetGenOauthConfig() (*oauth2.Config, error) {

	callback := self.router.CallbackHandler()
	res := &oauth2.Config{
		RedirectURL:  api_utils.GetPublicURL(self.config_obj, callback),
		ClientID:     self.authenticator.OauthClientId,
		ClientSecret: self.authenticator.OauthClientSecret,
		Scopes:       self.router.Scopes(),
	}

	self.Debug("OidcAuthenticator: OIDC configuration: %#v", res)
	return res, nil
}

// Ensure an XSRF protection for the auth flow by ensuring the
// callback is matched with the redirect by setting a cookie on the
// browser.
func generateStateOauthCookie(
	config_obj *config_proto.Config,
	w http.ResponseWriter) *http.Cookie {
	var expiration = utils.GetTime().Now().Add(time.Hour)

	b := make([]byte, 16)
	_, _ = rand.Read(b)
	state := base64.URLEncoding.EncodeToString(b)
	cookie := http.Cookie{
		Name:     "oauthstate",
		Path:     api_utils.GetBasePath(config_obj),
		Value:    state,
		Secure:   true,
		HttpOnly: true,
		Expires:  expiration}
	http.SetCookie(w, &cookie)
	return &cookie
}

func (self *OidcAuthenticator) oauthOidcLogin(provider ProviderInterface) http.Handler {

	return api_utils.HandlerFunc(nil,
		func(w http.ResponseWriter, r *http.Request) {
			// Create oauthState cookie
			oauthState, err := r.Cookie("oauthstate")
			if err != nil {
				oauthState = generateStateOauthCookie(self.config_obj, w)
			}

			// Needed for Okta to specify `prompt: login` to avoid consent
			// auth on each login.
			var options []oauth2.AuthCodeOption
			for k, v := range self.authenticator.OidcAuthUrlParams {
				options = append(options, oauth2.SetAuthURLParam(k, v))
			}

			url := provider.GetRedirectURL(options, oauthState.Value)

			self.Debug("OidcAuthenticator: Redirecting to: %#v", url)

			http.Redirect(w, r, url, http.StatusFound)
		})
}

func (self *OidcAuthenticator) Debug(message string, args ...interface{}) {
	if self.authenticator.OidcDebug {
		logging.GetLogger(self.config_obj, &logging.GUIComponent).
			Debug(message, args...)
	}
}

func (self *OidcAuthenticator) Error(message string, args ...interface{}) {
	logging.GetLogger(self.config_obj, &logging.GUIComponent).
		Error(message, args...)
}

func (self *OidcAuthenticator) oauthOidcCallback(
	provider ProviderInterface) http.Handler {
	return api_utils.HandlerFunc(nil,
		func(w http.ResponseWriter, r *http.Request) {
			self.Debug("OidcAuthenticator: Received OIDC Callback %#v", r)

			// Read oauthState from Cookie and make sure the state
			// that is passed back from the server are actually the
			// same.
			oauthState, _ := r.Cookie("oauthstate")
			if oauthState == nil || r.FormValue("state") != oauthState.Value {
				self.Error("invalid oauth state of OIDC: %v %v",
					oauthState, r.FormValue("state"))
				http.Redirect(w, r, api_utils.Homepage(self.config_obj),
					http.StatusTemporaryRedirect)
				return
			}

			ctx, err := ClientContext(r.Context(), self.config_obj,
				DefaultTransforms(self.config_obj, self.authenticator))
			if err != nil {
				self.Error("OidcAuthenticator: %v", err)
				http.Redirect(w, r, api_utils.Homepage(self.config_obj),
					http.StatusTemporaryRedirect)
				return
			}

			code := r.FormValue("code")
			cookie, claims, err := provider.GetJWT(ctx, code)
			if err != nil {
				self.Error("OidcAuthenticator: %v", err)
				http.Redirect(w, r, api_utils.Homepage(self.config_obj),
					http.StatusTemporaryRedirect)
				return
			}

			// Log a successful login.
			err = services.LogAudit(r.Context(),
				self.config_obj, claims.Username, "Login",
				ordereddict.NewDict().
					Set("remote", r.RemoteAddr).
					Set("authenticator", self.authenticator.Type).
					Set("url", r.URL.Path))
			if err != nil {
				self.Error("getSignedJWTTokenCookie LogAudit: Login %v %v",
					claims.Username, r.RemoteAddr)
			}

			self.Debug("oauthOidcCallback: Success! Setting cookie %#v", cookie)

			http.SetCookie(w, cookie)
			http.Redirect(w, r, api_utils.Homepage(self.config_obj),
				http.StatusTemporaryRedirect)
		})
}

func NewOidcAuthenticator(
	config_obj *config_proto.Config,
	authenticator *config_proto.Authenticator,
	router OidcRouter,
	claims_getter ClaimsGetter) *OidcAuthenticator {
	return &OidcAuthenticator{
		router:        router,
		claims_getter: claims_getter,
		config_obj:    config_obj,
		authenticator: authenticator,
	}
}
