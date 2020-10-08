/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package authenticators

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/gorilla/csrf"
	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	users "www.velocidex.com/golang/velociraptor/users"
)

const oauthGoogleUrlAPI = "https://www.googleapis.com/oauth2/v2/userinfo?access_token="

type GoogleAuthenticator struct{}

func (self *GoogleAuthenticator) AddHandlers(config_obj *config_proto.Config, mux *http.ServeMux) error {
	mux.Handle("/auth/google/login", oauthGoogleLogin(config_obj))
	mux.Handle("/auth/google/callback", oauthGoogleCallback(config_obj))

	installLogoff(config_obj, mux)

	return nil
}

func (self *GoogleAuthenticator) IsPasswordLess() bool {
	return true
}

// Check that the user is proerly authenticated.
func (self *GoogleAuthenticator) AuthenticateUserHandler(
	config_obj *config_proto.Config,
	parent http.Handler) http.Handler {

	return authenticateUserHandle(
		config_obj, parent, "/auth/google/login", "Google")
}

func oauthGoogleLogin(config_obj *config_proto.Config) http.Handler {
	authenticator := config_obj.GUI.Authenticator

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var googleOauthConfig = &oauth2.Config{
			RedirectURL:  config_obj.GUI.PublicUrl + "auth/google/callback",
			ClientID:     authenticator.OauthClientId,
			ClientSecret: authenticator.OauthClientSecret,
			Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email"},
			Endpoint:     google.Endpoint,
		}

		// Create oauthState cookie
		oauthState, err := r.Cookie("oauthstate")
		if err != nil {
			oauthState = generateStateOauthCookie(w)
		}

		u := googleOauthConfig.AuthCodeURL(oauthState.Value, oauth2.ApprovalForce)
		http.Redirect(w, r, u, http.StatusTemporaryRedirect)
	})
}

func generateStateOauthCookie(w http.ResponseWriter) *http.Cookie {
	var expiration = time.Now().Add(365 * 24 * time.Hour)

	b := make([]byte, 16)
	_, _ = rand.Read(b)
	state := base64.URLEncoding.EncodeToString(b)
	cookie := http.Cookie{Name: "oauthstate", Value: state, Expires: expiration}
	http.SetCookie(w, &cookie)

	return &cookie
}

func oauthGoogleCallback(config_obj *config_proto.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read oauthState from Cookie
		oauthState, _ := r.Cookie("oauthstate")

		if r.FormValue("state") != oauthState.Value {
			logging.GetLogger(config_obj, &logging.GUIComponent).
				Error("invalid oauth google state")
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		data, err := getUserDataFromGoogle(
			r.Context(), config_obj, r.FormValue("code"))
		if err != nil {
			logging.GetLogger(config_obj, &logging.GUIComponent).
				WithFields(logrus.Fields{
					"err": err,
				}).Error("getUserDataFromGoogle")
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		user_info := &api_proto.VelociraptorUser{}
		err = json.Unmarshal(data, &user_info)
		if err != nil {
			logging.GetLogger(config_obj, &logging.GUIComponent).
				WithFields(logrus.Fields{
					"err": err,
				}).Error("getUserDataFromGoogle")
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		// Create a new token object, specifying signing method and the claims
		// you would like it to contain.
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"user": user_info.Email,
			// Required re-auth after one day.
			"expires": float64(time.Now().AddDate(0, 0, 1).Unix()),
			"picture": user_info.Picture,
		})

		// Sign and get the complete encoded token as a string using the secret
		tokenString, err := token.SignedString(
			[]byte(config_obj.Frontend.PrivateKey))
		if err != nil {
			logging.GetLogger(config_obj, &logging.GUIComponent).
				WithFields(logrus.Fields{
					"err": err,
				}).Error("getUserDataFromGoogle")
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		// Set the cookie and redirect.
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

func getUserDataFromGoogle(
	ctx context.Context,
	config_obj *config_proto.Config,
	code string) ([]byte, error) {
	authenticator := config_obj.GUI.Authenticator
	// Use code to get token and get user info from Google.
	var googleOauthConfig = &oauth2.Config{
		RedirectURL:  config_obj.GUI.PublicUrl + "auth/google/callback",
		ClientID:     authenticator.OauthClientId,
		ClientSecret: authenticator.OauthClientSecret,
		Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email"},
		Endpoint:     google.Endpoint,
	}

	token, err := googleOauthConfig.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("code exchange wrong: %s", err.Error())
	}
	response, err := http.Get(oauthGoogleUrlAPI + token.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed getting user info: %s", err.Error())
	}
	defer response.Body.Close()

	contents, err := ioutil.ReadAll(
		io.LimitReader(response.Body, constants.MAX_MEMORY))
	if err != nil {
		return nil, fmt.Errorf("failed read response: %s", err.Error())
	}
	return contents, nil
}

func installLogoff(config_obj *config_proto.Config, mux *http.ServeMux) {
	// On logoff just clear the cookie and redirect.
	mux.Handle("/logoff", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params := r.URL.Query()
		old_username, ok := params["username"]
		if ok && len(old_username) == 1 {
			logger := logging.GetLogger(config_obj, &logging.Audit)
			logger.Info("Logging off %v", old_username[0])
		}
		http.SetCookie(w, &http.Cookie{
			Name:    "VelociraptorAuth",
			Path:    "/",
			Value:   "",
			Expires: time.Unix(0, 0),
		})
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	}))
}

func authenticateUserHandle(config_obj *config_proto.Config,
	parent http.Handler, login_url string, provider string) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-CSRF-Token", csrf.Token(r))

		// Reject by redirecting to the loging handler.
		reject := func(err error) {
			logger := logging.GetLogger(config_obj, &logging.Audit)
			logger.WithFields(logrus.Fields{
				"remote": r.RemoteAddr,
			}).Error("OAuth2 Redirect")

			// Not authorized - redirect to logon screen.
			http.Redirect(w, r, login_url, http.StatusTemporaryRedirect)
		}

		// We store the user name and their details in a local
		// cookie. It is stored as a JWT so we can trust it.
		auth_cookie, err := r.Cookie("VelociraptorAuth")
		if err != nil {
			reject(err)
			return
		}

		// Parse the JWT.
		token, err := jwt.Parse(
			auth_cookie.Value,
			func(token *jwt.Token) (interface{}, error) {
				_, ok := token.Method.(*jwt.SigningMethodHMAC)
				if !ok {
					return nil, errors.New("invalid signing method")
				}
				return []byte(config_obj.Frontend.PrivateKey), nil
			})
		if err != nil {
			reject(err)
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok || !token.Valid {
			reject(errors.New("token not valid"))
			return
		}

		// Record the username for handlers lower in the
		// stack.
		username, pres := claims["user"].(string)
		if !pres {
			reject(errors.New("username not present"))
			return
		}

		// Check if the claim is too old.
		expires, pres := claims["expires"].(float64)
		if !pres {
			reject(errors.New("expires field not present in JWT"))
			return
		}

		if expires < float64(time.Now().Unix()) {
			reject(errors.New("the JWT is expired - reauthenticate"))
			return
		}

		picture, _ := claims["picture"].(string)

		// Now check if the user is allowed to log in.
		user_record, err := users.GetUser(config_obj, username)
		if err != nil {
			reject(errors.New("Invalid user"))
			return
		}

		// Must have at least reader permission.
		perm, err := acls.CheckAccess(config_obj, username, acls.READ_RESULTS)
		if !perm || err != nil || user_record.Locked || user_record.Name != username {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)

			fmt.Fprintf(w, `
<html><body>
Authorization failed. You are not registered on this system as %v.
Contact your system administrator to get an account, or click here
to log in again:

      <a href="%s" style="text-transform:none">
        Login with %s
      </a>
</body></html>
`, username, login_url, provider)

			logging.GetLogger(config_obj, &logging.Audit).
				WithFields(logrus.Fields{
					"user":   username,
					"remote": r.RemoteAddr,
					"method": r.Method,
				}).Error("User rejected by GUI")
			return
		}

		// Checking is successful - user authorized. Here we
		// build a token to pass to the underlying GRPC
		// service with metadata about the user.
		user_info := &api_proto.VelociraptorUser{
			Name:    username,
			Picture: picture,
		}

		// Must use json encoding because grpc can not handle
		// binary data in metadata.
		serialized, _ := json.Marshal(user_info)
		ctx := context.WithValue(
			r.Context(), constants.GRPC_USER_CONTEXT, string(serialized))

		// Need to call logging after auth so it can access
		// the contextKeyUser value in the context.
		GetLoggingHandler(config_obj)(parent).ServeHTTP(
			w, r.WithContext(ctx))
	})
}
