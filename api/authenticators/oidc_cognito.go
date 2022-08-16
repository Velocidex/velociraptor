package authenticators

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	oidc "github.com/coreos/go-oidc"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
)

// AWS Cognito needs a special authenticator because they do not
// follow the oauth2 spec properly. See
// https://github.com/coreos/go-oidc/pull/249
type OidcAuthenticatorCognito struct {
	OidcAuthenticator
}

func (self *OidcAuthenticatorCognito) AddHandlers(mux *http.ServeMux) error {
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

func (self *OidcAuthenticatorCognito) oauthOidcCallback(
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
		userInfo, err := getUserInfo(
			r.Context(), provider, oauth2.StaticTokenSource(oauthToken))
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

func init() {
	RegisterAuthenticator("oidc-cognito", func(config_obj *config_proto.Config,
		auth_config *config_proto.Authenticator) (Authenticator, error) {
		return &OidcAuthenticatorCognito{OidcAuthenticator{
			config_obj:    config_obj,
			authenticator: auth_config,
		}}, nil
	})
}

// The following is taken from https://github.com/pomerium/pomerium/blob/0b0fba06b3374557ed7427d165190570ce4997f1/internal/identity/oidc/userinfo.go

// getUserInfo gets the user info for OIDC. We wrap the underlying call because AWS Cognito chose to violate the spec
// and return data in an invalid format. By using our own custom http client, we're able to modify the response to
// make it compliant, and then the rest of the library works as expected.
func getUserInfo(ctx context.Context, provider *oidc.Provider, tokenSource oauth2.TokenSource) (*oidc.UserInfo, error) {
	originalClient := http.DefaultClient
	if c, ok := ctx.Value(oauth2.HTTPClient).(*http.Client); ok {
		originalClient = c
	}

	client := new(http.Client)
	*client = *originalClient
	client.Transport = &userInfoRoundTripper{underlying: client.Transport}

	ctx = context.WithValue(ctx, oauth2.HTTPClient, client)
	return provider.UserInfo(ctx, tokenSource)
}

type userInfoRoundTripper struct {
	underlying http.RoundTripper
}

func (transport *userInfoRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	underlying := transport.underlying
	if underlying == nil {
		underlying = http.DefaultTransport
	}

	res, err := underlying.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	bs, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	var userInfo map[string]interface{}
	if err := json.Unmarshal(bs, &userInfo); err == nil {
		// AWS Cognito returns email_verified as a string, so we'll make it a bool
		if ev, ok := userInfo["email_verified"]; ok {
			userInfo["email_verified"], _ = strconv.ParseBool(fmt.Sprint(ev))
		}

		// Some providers (ping) have a "mail" claim instead of "email"
		email, mail := userInfo["email"], userInfo["mail"]
		if email == nil && mail != nil && strings.Contains(fmt.Sprint(mail), "@") {
			userInfo["email"] = mail
		}

		bs, _ = json.Marshal(userInfo)
	}

	res.Body = io.NopCloser(bytes.NewReader(bs))
	return res, nil
}
