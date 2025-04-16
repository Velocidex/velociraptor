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
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/microsoft"
	"www.velocidex.com/golang/velociraptor/acls"
	api_utils "www.velocidex.com/golang/velociraptor/api/utils"
	utils "www.velocidex.com/golang/velociraptor/api/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
)

type AzureUser struct {
	Mail    string `json:"userPrincipalName"`
	Name    string `json:"displayName"`
	Picture string `json:"picture"`
}

type AzureAuthenticator struct {
	config_obj       *config_proto.Config
	authenticator    *config_proto.Authenticator
	base, public_url string
}

// The URL that will be used to log in.
func (self *AzureAuthenticator) LoginURL() string {
	return "/auth/azure/login"
}

func (self *AzureAuthenticator) IsPasswordLess() bool {
	return true
}

func (self *AzureAuthenticator) RequireClientCerts() bool {
	return false
}

func (self *AzureAuthenticator) AuthRedirectTemplate() string {
	return self.authenticator.AuthRedirectTemplate
}

func (self *AzureAuthenticator) AddHandlers(mux *api_utils.ServeMux) error {
	mux.Handle(api_utils.GetBasePath(self.config_obj, self.LoginURL()),
		IpFilter(self.config_obj, self.oauthAzureLogin()))

	mux.Handle(api_utils.GetBasePath(self.config_obj, "/auth/azure/callback"),
		IpFilter(self.config_obj, self.oauthAzureCallback()))

	return nil
}

func (self *AzureAuthenticator) AddLogoff(mux *api_utils.ServeMux) error {
	installLogoff(self.config_obj, mux)
	return nil
}

// Check that the user is proerly authenticated.
func (self *AzureAuthenticator) AuthenticateUserHandler(
	parent http.Handler,
	permission acls.ACL_PERMISSION,
) http.Handler {

	return authenticateUserHandle(
		self.config_obj, permission,
		func(w http.ResponseWriter, r *http.Request, err error, username string) {
			reject_with_username(self.config_obj, w, r, err, username,
				self.LoginURL(), "Microsoft O365/Azure AD")
		},
		parent)
}

func (self *AzureAuthenticator) GetGenOauthConfig() (*oauth2.Config, error) {
	return &oauth2.Config{
		RedirectURL:  api_utils.GetPublicURL(self.config_obj, "/auth/azure/callback"),
		ClientID:     self.authenticator.OauthClientId,
		ClientSecret: self.authenticator.OauthClientSecret,
		Scopes:       []string{"User.Read"},
		Endpoint:     microsoft.AzureADEndpoint(self.authenticator.Tenant),
	}, nil
}

func (self *AzureAuthenticator) oauthAzureLogin() http.Handler {
	return api_utils.HandlerFunc(nil,
		func(w http.ResponseWriter, r *http.Request) {
			azureOauthConfig, _ := self.GetGenOauthConfig()

			// Create oauthState cookie
			oauthState, err := r.Cookie("oauthstate")
			if err != nil {
				oauthState = generateStateOauthCookie(self.config_obj, w)
			}

			u := azureOauthConfig.AuthCodeURL(oauthState.Value)
			http.Redirect(w, r, u, http.StatusTemporaryRedirect)
		})
}

func (self *AzureAuthenticator) oauthAzureCallback() http.Handler {
	return api_utils.HandlerFunc(nil,
		func(w http.ResponseWriter, r *http.Request) {
			// Read oauthState from Cookie
			oauthState, _ := r.Cookie("oauthstate")

			if oauthState == nil || r.FormValue("state") != oauthState.Value {
				logging.GetLogger(self.config_obj, &logging.GUIComponent).
					Error("invalid oauth azure state")
				http.Redirect(w, r, utils.Homepage(self.config_obj),
					http.StatusTemporaryRedirect)
				return
			}

			user_info, err := self.getUserDataFromAzure(
				r.Context(), r.FormValue("code"))
			if err != nil {
				logging.GetLogger(self.config_obj, &logging.GUIComponent).
					WithFields(logrus.Fields{
						"err": err.Error(),
					}).Error("getUserDataFromAzure")
				http.Redirect(w, r, utils.Homepage(self.config_obj),
					http.StatusTemporaryRedirect)
				return
			}

			// Create a new token object, specifying signing method and the claims
			// you would like it to contain.
			cookie, err := getSignedJWTTokenCookie(
				self.config_obj, self.authenticator,
				&Claims{
					Username: user_info.Mail,
				})
			if err != nil {
				logging.GetLogger(self.config_obj, &logging.GUIComponent).
					WithFields(logrus.Fields{
						"err": err.Error(),
					}).Error("getUserDataFromAzure")
				http.Redirect(w, r, utils.Homepage(self.config_obj),
					http.StatusTemporaryRedirect)
				return
			}

			http.SetCookie(w, cookie)
			http.Redirect(w, r, utils.Homepage(self.config_obj),
				http.StatusTemporaryRedirect)
		})
}

func (self *AzureAuthenticator) getUserDataFromAzure(
	ctx context.Context, code string) (*AzureUser, error) {

	// Use code to get token and get user info from Azure.
	azureOauthConfig, err := self.GetGenOauthConfig()
	if err != nil {
		return nil, err
	}

	token, err := azureOauthConfig.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("code exchange wrong: %s", err.Error())
	}

	response, err := azureOauthConfig.Client(ctx, token).Get(
		"https://graph.microsoft.com/v1.0/me/")
	if err != nil {
		return nil, fmt.Errorf("failed getting user info: %s", err.Error())
	}
	defer response.Body.Close()

	contents, err := ioutil.ReadAll(
		io.LimitReader(response.Body, constants.MAX_MEMORY))
	if err != nil {
		return nil, fmt.Errorf("failed read response: %s", err.Error())
	}

	user_info := &AzureUser{}
	err = json.Unmarshal(contents, &user_info)
	if err != nil {
		return nil, err
	}

	username := user_info.Mail
	if username != "" {
		setUserPicture(ctx, username, self.getAzurePicture(ctx, token))
	}

	user_info.Picture = "" // Server will fill it from the user record anyway.

	return user_info, nil
}

// Best effort - if anything fails we just dont show the picture.
func (self *AzureAuthenticator) getAzurePicture(
	ctx context.Context, token *oauth2.Token) string {
	azureOauthConfig, err := self.GetGenOauthConfig()
	if err != nil {
		return ""
	}

	response, err := azureOauthConfig.Client(ctx, token).Get(
		"https://graph.microsoft.com/v1.0/me/photos/48x48/$value")
	if err != nil {
		return ""
	}
	defer response.Body.Close()

	data, _ := ioutil.ReadAll(response.Body)

	return fmt.Sprintf("data:image/jpeg;base64,%v",
		base64.StdEncoding.EncodeToString(data))
}
