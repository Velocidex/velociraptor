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

	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/microsoft"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
)

type AzureUser struct {
	Mail  string `json:"userPrincipalName"`
	Name  string `json:"displayName"`
	Token string `json:"token"`
}

type AzureAuthenticator struct {
	config_obj    *config_proto.Config
	authenticator *config_proto.Authenticator
}

// The URL that will be used to log in.
func (self *AzureAuthenticator) LoginURL() string {
	return self.config_obj.GUI.PublicUrl + "auth/azure/login"
}

func (self *AzureAuthenticator) IsPasswordLess() bool {
	return true
}

func (self *AzureAuthenticator) AddHandlers(mux *http.ServeMux) error {
	mux.Handle("/auth/azure/login", self.oauthAzureLogin())
	mux.Handle("/auth/azure/callback", self.oauthAzureCallback())
	mux.Handle("/auth/azure/picture", self.oauthAzurePicture())
	return nil
}

func (self *AzureAuthenticator) AddLogoff(mux *http.ServeMux) error {
	installLogoff(self.config_obj, mux)
	return nil
}

// Check that the user is proerly authenticated.
func (self *AzureAuthenticator) AuthenticateUserHandler(
	parent http.Handler) http.Handler {

	return authenticateUserHandle(
		self.config_obj,
		func(w http.ResponseWriter, r *http.Request, err error, username string) {
			reject_with_username(self.config_obj, w, r, err, username,
				"/auth/azure/login", "Microsoft O365/Azure AD")
		},
		parent)
}

func (self *AzureAuthenticator) oauthAzureLogin() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var azureOauthConfig = &oauth2.Config{
			RedirectURL:  self.config_obj.GUI.PublicUrl + "auth/azure/callback",
			ClientID:     self.authenticator.OauthClientId,
			ClientSecret: self.authenticator.OauthClientSecret,
			Scopes:       []string{"User.Read"},
			Endpoint:     microsoft.AzureADEndpoint(self.authenticator.Tenant),
		}

		// Create oauthState cookie
		oauthState, err := r.Cookie("oauthstate")
		if err != nil {
			oauthState = generateStateOauthCookie(w)
		}

		u := azureOauthConfig.AuthCodeURL(oauthState.Value)
		http.Redirect(w, r, u, http.StatusTemporaryRedirect)
	})
}

func (self *AzureAuthenticator) oauthAzureCallback() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read oauthState from Cookie
		oauthState, _ := r.Cookie("oauthstate")

		if r.FormValue("state") != oauthState.Value {
			logging.GetLogger(self.config_obj, &logging.GUIComponent).
				Error("invalid oauth azure state")
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		user_info, err := self.getUserDataFromAzure(
			r.Context(), r.FormValue("code"))
		if err != nil {
			logging.GetLogger(self.config_obj, &logging.GUIComponent).
				WithFields(logrus.Fields{
					"err": err.Error(),
				}).Error("getUserDataFromAzure")
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		// Create a new token object, specifying signing method and the claims
		// you would like it to contain.
		cookie, err := getSignedJWTTokenCookie(
			self.config_obj, self.authenticator,
			&Claims{
				Username: user_info.Mail,
				Picture:  "/auth/azure/picture",
				Token:    user_info.Token,
			})
		if err != nil {
			logging.GetLogger(self.config_obj, &logging.GUIComponent).
				WithFields(logrus.Fields{
					"err": err.Error(),
				}).Error("getUserDataFromAzure")
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		http.SetCookie(w, cookie)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	})
}

func (self *AzureAuthenticator) getAzureOauthConfig() *oauth2.Config {
	return &oauth2.Config{
		RedirectURL:  self.config_obj.GUI.PublicUrl + "auth/azure/callback",
		ClientID:     self.authenticator.OauthClientId,
		ClientSecret: self.authenticator.OauthClientSecret,
		Scopes:       []string{"User.Read"},
		Endpoint:     microsoft.AzureADEndpoint(self.authenticator.Tenant),
	}
}

func (self *AzureAuthenticator) getUserDataFromAzure(
	ctx context.Context, code string) (*AzureUser, error) {

	// Use code to get token and get user info from Azure.
	azureOauthConfig := self.getAzureOauthConfig()

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

	serialized, err := json.Marshal(token)
	if err != nil {
		return nil, err
	}

	user_info := &AzureUser{}
	err = json.Unmarshal(contents, &user_info)
	if err != nil {
		return nil, err
	}

	// Store the oauth token in the JWT so that we can store it in
	// the cookie. We will use the cookie value to retrieve the
	// picture using some more Azure APIs.
	user_info.Token = string(serialized)

	return user_info, nil
}

// Get the token from the cookie and request the picture from Azure
func (self *AzureAuthenticator) oauthAzurePicture() http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		reject := func(err error) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
		}

		claims, err := getDetailsFromCookie(self.config_obj, r)
		if err != nil {
			reject(err)
			return
		}

		oauth_token := &oauth2.Token{}
		err = json.Unmarshal([]byte(claims.Token), &oauth_token)
		if err != nil {
			reject(err)
			return
		}

		azureOauthConfig := self.getAzureOauthConfig()
		response, err := azureOauthConfig.Client(r.Context(), oauth_token).Get(
			"https://graph.microsoft.com/v1.0/me/photos/48x48/$value")
		if err != nil {
			reject(fmt.Errorf("failed getting photo: %v", err))
			return
		}
		defer response.Body.Close()

		_, err = io.Copy(w, response.Body)
		if err != nil {
			reject(fmt.Errorf("failed getting photo: %v", err))
			return
		}

	})
}
