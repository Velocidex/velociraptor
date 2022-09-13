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
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
)

type GitHubUser struct {
	Login     string `json:"login"`
	AvatarUrl string `json:"avatar_url"`
}

type GitHubAuthenticator struct {
	config_obj       *config_proto.Config
	authenticator    *config_proto.Authenticator
	base, public_url string
}

// The URL that will be used to log in.
func (self *GitHubAuthenticator) LoginURL() string {
	return self.base + "auth/github/login"
}

func (self *GitHubAuthenticator) IsPasswordLess() bool {
	return true
}

func (self *GitHubAuthenticator) AddHandlers(mux *http.ServeMux) error {
	mux.Handle(self.base+"auth/github/login", self.oauthGithubLogin())
	mux.Handle(self.base+"auth/github/callback", self.oauthGithubCallback())
	return nil
}

func (self *GitHubAuthenticator) AddLogoff(mux *http.ServeMux) error {
	installLogoff(self.config_obj, mux)
	return nil
}

// Check that the user is proerly authenticated.
func (self *GitHubAuthenticator) AuthenticateUserHandler(
	parent http.Handler) http.Handler {

	return authenticateUserHandle(
		self.config_obj,
		func(w http.ResponseWriter, r *http.Request, err error, username string) {
			reject_with_username(self.config_obj, w, r, err, username,
				self.base+"auth/github/login", "Github")
		},
		parent)
}

func (self *GitHubAuthenticator) oauthGithubLogin() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var githubOauthConfig = &oauth2.Config{
			RedirectURL: self.public_url +
				strings.TrimPrefix(self.base, "/") + "auth/github/callback",
			ClientID:     self.authenticator.OauthClientId,
			ClientSecret: self.authenticator.OauthClientSecret,
			Scopes:       []string{"user:email"},
			Endpoint:     github.Endpoint,
		}

		// Create oauthState cookie
		oauthState, err := r.Cookie("oauthstate")
		if err != nil {
			oauthState = generateStateOauthCookie(w)
		}

		u := githubOauthConfig.AuthCodeURL(oauthState.Value, oauth2.ApprovalForce)
		http.Redirect(w, r, u, http.StatusTemporaryRedirect)
	})
}

func (self *GitHubAuthenticator) oauthGithubCallback() http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read oauthState from Cookie
		oauthState, _ := r.Cookie("oauthstate")

		if oauthState == nil || r.FormValue("state") != oauthState.Value {
			logging.GetLogger(self.config_obj, &logging.GUIComponent).
				Error("invalid oauth github state")
			http.Redirect(w, r, self.base, http.StatusTemporaryRedirect)
			return
		}

		data, err := self.getUserDataFromGithub(r.Context(), r.FormValue("code"))
		if err != nil {
			logging.GetLogger(self.config_obj, &logging.GUIComponent).
				WithFields(logrus.Fields{
					"err": err.Error(),
				}).Error("getUserDataFromGithub")
			http.Redirect(w, r, self.base, http.StatusTemporaryRedirect)
			return
		}

		user_info := &GitHubUser{}
		err = json.Unmarshal(data, &user_info)
		if err != nil {
			logging.GetLogger(self.config_obj, &logging.GUIComponent).
				WithFields(logrus.Fields{
					"err": err.Error(),
				}).Error("getUserDataFromGithub")
			http.Redirect(w, r, self.base, http.StatusTemporaryRedirect)
			return
		}

		cookie, err := getSignedJWTTokenCookie(
			self.config_obj, self.authenticator,
			&Claims{
				Username: user_info.Login,
				Picture:  user_info.AvatarUrl,
			})
		if err != nil {
			logging.GetLogger(self.config_obj, &logging.GUIComponent).
				WithFields(logrus.Fields{
					"err": err.Error(),
				}).Error("getUserDataFromGithub")
			http.Redirect(w, r, self.base, http.StatusTemporaryRedirect)
			return
		}

		http.SetCookie(w, cookie)
		http.Redirect(w, r, self.base, http.StatusTemporaryRedirect)
	})
}

func (self *GitHubAuthenticator) getUserDataFromGithub(
	ctx context.Context, code string) ([]byte, error) {

	// Use code to get token and get user info from GitHub.
	var githubOauthConfig = &oauth2.Config{
		RedirectURL: self.public_url +
			strings.TrimPrefix(self.base, "/") + "auth/github/callback",
		ClientID:     self.authenticator.OauthClientId,
		ClientSecret: self.authenticator.OauthClientSecret,
		Scopes:       []string{},
		Endpoint:     github.Endpoint,
	}

	token, err := githubOauthConfig.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("code exchange wrong: %s", err.Error())
	}

	response, err := githubOauthConfig.Client(ctx, token).Get("https://api.github.com/user")
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
