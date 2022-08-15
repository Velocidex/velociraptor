/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2022 Rapid7 Inc.

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

	"github.com/gorilla/csrf"
	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/gui/velociraptor"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
)

const oauthGoogleUrlAPI = "https://www.googleapis.com/oauth2/v2/userinfo?access_token="

type GoogleAuthenticator struct {
	config_obj    *config_proto.Config
	authenticator *config_proto.Authenticator
}

func (self *GoogleAuthenticator) LoginHandler() string {
	return "/auth/google/login"
}

// The URL that will be used to log in.
func (self *GoogleAuthenticator) LoginURL() string {
	return self.config_obj.GUI.PublicUrl + "auth/google/login"
}

func (self *GoogleAuthenticator) CallbackHandler() string {
	return "/auth/google/callback"
}

func (self *GoogleAuthenticator) CallbackURL() string {
	return self.config_obj.GUI.PublicUrl + "auth/google/callback"
}

func (self *GoogleAuthenticator) ProviderName() string {
	return "Google"
}

func (self *GoogleAuthenticator) AddHandlers(mux *http.ServeMux) error {
	mux.Handle(self.LoginHandler(), self.oauthGoogleLogin())
	mux.Handle(self.CallbackHandler(), self.oauthGoogleCallback())
	return nil
}

func (self *GoogleAuthenticator) AddLogoff(mux *http.ServeMux) error {
	installLogoff(self.config_obj, mux)
	return nil
}

func (self *GoogleAuthenticator) IsPasswordLess() bool {
	return true
}

// Check that the user is proerly authenticated.
func (self *GoogleAuthenticator) AuthenticateUserHandler(
	parent http.Handler) http.Handler {

	return authenticateUserHandle(
		self.config_obj,
		func(w http.ResponseWriter, r *http.Request, err error, username string) {
			reject_with_username(self.config_obj, w, r, err, username,
				self.LoginURL(), self.ProviderName())
		},
		parent)
}

func (self *GoogleAuthenticator) oauthGoogleLogin() http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var googleOauthConfig = &oauth2.Config{
			RedirectURL:  self.CallbackURL(),
			ClientID:     self.authenticator.OauthClientId,
			ClientSecret: self.authenticator.OauthClientSecret,
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
	// Do not expire from the browser - we will expire it anyway.
	var expiration = time.Now().Add(365 * 24 * time.Hour)

	b := make([]byte, 16)
	_, _ = rand.Read(b)
	state := base64.URLEncoding.EncodeToString(b)
	cookie := http.Cookie{
		Name:     "oauthstate",
		Value:    state,
		Secure:   true,
		HttpOnly: true,
		Expires:  expiration}
	http.SetCookie(w, &cookie)

	return &cookie
}

func (self *GoogleAuthenticator) oauthGoogleCallback() http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read oauthState from Cookie
		oauthState, _ := r.Cookie("oauthstate")

		if r.FormValue("state") != oauthState.Value {
			logging.GetLogger(self.config_obj, &logging.GUIComponent).
				Error("invalid oauth google state")
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		data, err := self.getUserDataFromGoogle(r.Context(), r.FormValue("code"))
		if err != nil {
			logging.GetLogger(self.config_obj, &logging.GUIComponent).
				WithFields(logrus.Fields{
					"err": err.Error(),
				}).Error("getUserDataFromGoogle")
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		user_info := &api_proto.VelociraptorUser{}
		err = json.Unmarshal(data, &user_info)
		if err != nil {
			logging.GetLogger(self.config_obj, &logging.GUIComponent).
				WithFields(logrus.Fields{
					"err": err.Error(),
				}).Error("getUserDataFromGoogle")
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		// Sign and get the complete encoded token as a string using the secret
		cookie, err := getSignedJWTTokenCookie(
			self.config_obj, self.authenticator,
			&Claims{
				Username: user_info.Email,
				Picture:  user_info.Picture,
			})
		if err != nil {
			logging.GetLogger(self.config_obj, &logging.GUIComponent).
				WithFields(logrus.Fields{
					"err": err.Error(),
				}).Error("getUserDataFromGoogle")
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		http.SetCookie(w, cookie)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	})
}

func (self *GoogleAuthenticator) getUserDataFromGoogle(
	ctx context.Context, code string) ([]byte, error) {

	// Use code to get token and get user info from Google.
	var googleOauthConfig = &oauth2.Config{
		RedirectURL:  self.CallbackURL(),
		ClientID:     self.authenticator.OauthClientId,
		ClientSecret: self.authenticator.OauthClientSecret,
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
	base := config_obj.GUI.BasePath
	mux.Handle(base+"/app/logoff.html",
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			params := r.URL.Query()
			old_username, ok := params["username"]
			username := ""
			if ok && len(old_username) == 1 {
				logger := logging.GetLogger(config_obj, &logging.Audit)
				logger.Info("Logging off %v", old_username[0])
				username = old_username[0]
			}

			// Clear the cookie
			http.SetCookie(w, &http.Cookie{
				Name:     "VelociraptorAuth",
				Path:     "/",
				Value:    "",
				Secure:   true,
				HttpOnly: true,
				Expires:  time.Unix(0, 0),
			})

			//w.Header().Set("Content-Type", "text/html; charset=utf-8")
			//w.WriteHeader(http.StatusUnauthorized)

			renderLogoffMessage(w, username)
		}))
}

func authenticateUserHandle(
	config_obj *config_proto.Config,
	reject_cb func(w http.ResponseWriter, r *http.Request,
		err error, username string),
	parent http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-CSRF-Token", csrf.Token(r))

		claims, err := getDetailsFromCookie(config_obj, r)
		if err != nil {
			reject_cb(w, r, err, claims.Username)
			return
		}

		username := claims.Username

		// Now check if the user is allowed to log in.
		users := services.GetUserManager()
		user_record, err := users.GetUser(username)
		if err != nil {
			reject_cb(w, r, errors.New("Invalid user"), username)
			return
		}

		// Must have at least reader permission.
		perm, err := acls.CheckAccess(config_obj, username, acls.READ_RESULTS)
		if !perm || err != nil || user_record.Locked || user_record.Name != username {
			reject_cb(w, r, errors.New("Insufficient permissions"), username)
			return
		}

		// Checking is successful - user authorized. Here we
		// build a token to pass to the underlying GRPC
		// service with metadata about the user.
		user_info := &api_proto.VelociraptorUser{
			Name:    username,
			Picture: claims.Picture,
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

func reject_with_username(
	config_obj *config_proto.Config,
	w http.ResponseWriter, r *http.Request,
	err error, username, login_url, provider string) {
	logger := logging.GetLogger(config_obj, &logging.Audit)
	// Log into the audit log.
	logger.WithFields(logrus.Fields{
		"user":   username,
		"remote": r.RemoteAddr,
		"method": r.Method,
		"url":    r.URL,
		"err":    err.Error(),
	}).Error("User rejected by GUI")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)

	renderRejectionMessage(w, username, []velociraptor.AuthenticatorInfo{
		{
			LoginURL:     login_url,
			ProviderName: provider,
		},
	})
}
