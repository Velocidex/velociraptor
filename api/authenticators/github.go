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
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/logging"
)

type GitHubUser struct {
	Login     string `json:"login"`
	AvatarUrl string `json:"avatar_url"`
}

type GitHubAuthenticator struct{}

func (self *GitHubAuthenticator) IsPasswordLess() bool {
	return true
}

func (self *GitHubAuthenticator) AddHandlers(config_obj *config_proto.Config, mux *http.ServeMux) error {
	mux.Handle("/auth/github/login", oauthGithubLogin(config_obj))
	mux.Handle("/auth/github/callback", oauthGithubCallback(config_obj))

	installLogoff(config_obj, mux)
	return nil
}

// Check that the user is proerly authenticated.
func (self *GitHubAuthenticator) AuthenticateUserHandler(
	config_obj *config_proto.Config,
	parent http.Handler) http.Handler {

	return authenticateUserHandle(
		config_obj, parent, "/auth/github/login", "GitHub")
}

func oauthGithubLogin(config_obj *config_proto.Config) http.Handler {
	authenticator := config_obj.GUI.Authenticator

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var githubOauthConfig = &oauth2.Config{
			RedirectURL:  config_obj.GUI.PublicUrl + "auth/github/callback",
			ClientID:     authenticator.OauthClientId,
			ClientSecret: authenticator.OauthClientSecret,
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

func oauthGithubCallback(config_obj *config_proto.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read oauthState from Cookie
		oauthState, _ := r.Cookie("oauthstate")

		if r.FormValue("state") != oauthState.Value {
			logging.GetLogger(config_obj, &logging.GUIComponent).
				Error("invalid oauth github state")
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		data, err := getUserDataFromGithub(
			r.Context(), config_obj, r.FormValue("code"))
		if err != nil {
			logging.GetLogger(config_obj, &logging.GUIComponent).
				WithFields(logrus.Fields{
					"err": err,
				}).Error("getUserDataFromGithub")
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		user_info := &GitHubUser{}
		err = json.Unmarshal(data, &user_info)
		if err != nil {
			logging.GetLogger(config_obj, &logging.GUIComponent).
				WithFields(logrus.Fields{
					"err": err,
				}).Error("getUserDataFromGithub")
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		// Create a new token object, specifying signing method and the claims
		// you would like it to contain.
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"user": user_info.Login,
			// Required re-auth after one day.
			"expires": float64(time.Now().AddDate(0, 0, 1).Unix()),
			"picture": user_info.AvatarUrl,
		})

		// Sign and get the complete encoded token as a string using the secret
		tokenString, err := token.SignedString(
			[]byte(config_obj.Frontend.PrivateKey))
		if err != nil {
			logging.GetLogger(config_obj, &logging.GUIComponent).
				WithFields(logrus.Fields{
					"err": err,
				}).Error("getUserDataFromGithub")
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

func getUserDataFromGithub(
	ctx context.Context,
	config_obj *config_proto.Config, code string) ([]byte, error) {
	authenticator := config_obj.GUI.Authenticator

	// Use code to get token and get user info from GitHub.
	var githubOauthConfig = &oauth2.Config{
		RedirectURL:  config_obj.GUI.PublicUrl + "auth/github/callback",
		ClientID:     authenticator.OauthClientId,
		ClientSecret: authenticator.OauthClientSecret,
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
