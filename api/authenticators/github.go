/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

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
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"www.velocidex.com/golang/velociraptor/acls"
	api_utils "www.velocidex.com/golang/velociraptor/api/utils"
	utils "www.velocidex.com/golang/velociraptor/api/utils"
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
	return "/auth/github/login"
}

func (self *GitHubAuthenticator) CallbackHandler() string {
	return "/auth/github/callback"
}

func (self *GitHubAuthenticator) IsPasswordLess() bool {
	return true
}

func (self *GitHubAuthenticator) RequireClientCerts() bool {
	return false
}

func (self *GitHubAuthenticator) AuthRedirectTemplate() string {
	return self.authenticator.AuthRedirectTemplate
}

func (self *GitHubAuthenticator) AddHandlers(mux *api_utils.ServeMux) error {
	mux.Handle(api_utils.GetBasePath(self.config_obj, self.LoginURL()),
		IpFilter(self.config_obj, self.oauthGithubLogin()))
	mux.Handle(api_utils.GetBasePath(self.config_obj, self.CallbackHandler()),
		IpFilter(self.config_obj, self.oauthGithubCallback()))
	return nil
}

func (self *GitHubAuthenticator) AddLogoff(mux *api_utils.ServeMux) error {
	installLogoff(self.config_obj, mux)
	return nil
}

// Check that the user is proerly authenticated.
func (self *GitHubAuthenticator) AuthenticateUserHandler(
	parent http.Handler,
	permission acls.ACL_PERMISSION,
) http.Handler {

	return authenticateUserHandle(
		self.config_obj, permission,
		func(w http.ResponseWriter, r *http.Request, err error, username string) {
			reject_with_username(self.config_obj, w, r, err, username,
				utils.Join(self.base, "/auth/github/login"), "Github")
		},
		parent)
}

func (self *GitHubAuthenticator) GetGenOauthConfig() (*oauth2.Config, error) {
	return &oauth2.Config{
		RedirectURL:  utils.GetPublicURL(self.config_obj, "/auth/github/callback"),
		ClientID:     self.authenticator.OauthClientId,
		ClientSecret: self.authenticator.OauthClientSecret,
		Scopes:       []string{"user:email"},
		Endpoint:     github.Endpoint,
	}, nil
}

func (self *GitHubAuthenticator) oauthGithubLogin() http.Handler {
	return api_utils.HandlerFunc(nil,
		func(w http.ResponseWriter, r *http.Request) {
			githubOauthConfig, _ := self.GetGenOauthConfig()

			// Create oauthState cookie
			oauthState, err := r.Cookie("oauthstate")
			if err != nil {
				oauthState = generateStateOauthCookie(self.config_obj, w)
			}

			u := githubOauthConfig.AuthCodeURL(oauthState.Value, oauth2.ApprovalForce)
			http.Redirect(w, r, u, http.StatusTemporaryRedirect)
		})
}

func (self *GitHubAuthenticator) oauthGithubCallback() http.Handler {

	return api_utils.HandlerFunc(nil,
		func(w http.ResponseWriter, r *http.Request) {
			// Read oauthState from Cookie
			oauthState, _ := r.Cookie("oauthstate")

			if oauthState == nil || r.FormValue("state") != oauthState.Value {
				logging.GetLogger(self.config_obj, &logging.GUIComponent).
					Error("invalid oauth github state")
				http.Redirect(w, r, utils.Homepage(self.config_obj),
					http.StatusTemporaryRedirect)
				return
			}

			formError := r.FormValue("error")
			if formError != "" {
				desc := r.FormValue("error_description")
				if desc != "" {
					formError = desc
				}
				logging.GetLogger(self.config_obj, &logging.GUIComponent).
					WithFields(logrus.Fields{
						"err": formError,
					}).Error("getUserDataFromGithub")
				http.Redirect(w, r, utils.Homepage(self.config_obj),
					http.StatusTemporaryRedirect)
				return
			}

			data, err := self.getUserDataFromGithub(r.Context(), r.FormValue("code"))
			if err != nil {
				logging.GetLogger(self.config_obj, &logging.GUIComponent).
					WithFields(logrus.Fields{
						"err": err.Error(),
					}).Error("getUserDataFromGithub")
				http.Redirect(w, r, utils.Homepage(self.config_obj),
					http.StatusTemporaryRedirect)
				return
			}

			user_info := &GitHubUser{}
			err = json.Unmarshal(data, &user_info)
			if err != nil {
				logging.GetLogger(self.config_obj, &logging.GUIComponent).
					WithFields(logrus.Fields{
						"err": err.Error(),
					}).Error("getUserDataFromGithub")
				http.Redirect(w, r, utils.Homepage(self.config_obj),
					http.StatusTemporaryRedirect)
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
				http.Redirect(w, r, utils.Homepage(self.config_obj),
					http.StatusTemporaryRedirect)
				return
			}

			http.SetCookie(w, cookie)
			http.Redirect(w, r, utils.Homepage(self.config_obj),
				http.StatusTemporaryRedirect)
		})
}

func (self *GitHubAuthenticator) getUserDataFromGithub(
	ctx context.Context, code string) ([]byte, error) {

	// Use code to get token and get user info from GitHub.
	githubOauthConfig, _ := self.GetGenOauthConfig()

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
