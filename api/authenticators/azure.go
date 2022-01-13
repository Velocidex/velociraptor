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
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	jwt "github.com/golang-jwt/jwt"
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

type AzureAuthenticator struct{}

func (self *AzureAuthenticator) IsPasswordLess() bool {
	return true
}

func (self *AzureAuthenticator) AddHandlers(config_obj *config_proto.Config, mux *http.ServeMux) error {
	mux.Handle("/auth/azure/login", oauthAzureLogin(config_obj))
	mux.Handle("/auth/azure/callback", oauthAzureCallback(config_obj))
	mux.Handle("/auth/azure/picture", oauthAzurePicture(config_obj))

	installLogoff(config_obj, mux)
	return nil
}

// Check that the user is proerly authenticated.
func (self *AzureAuthenticator) AuthenticateUserHandler(
	config_obj *config_proto.Config,
	parent http.Handler) http.Handler {

	return authenticateUserHandle(
		config_obj, parent, "/auth/azure/login", "Microsoft O365/Azure AD")
}

func oauthAzureLogin(config_obj *config_proto.Config) http.Handler {
	authenticator := config_obj.GUI.Authenticator

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var azureOauthConfig = &oauth2.Config{
			RedirectURL:  config_obj.GUI.PublicUrl + "auth/azure/callback",
			ClientID:     authenticator.OauthClientId,
			ClientSecret: authenticator.OauthClientSecret,
			Scopes:       []string{"User.Read"},
			Endpoint:     microsoft.AzureADEndpoint(authenticator.Tenant),
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

func oauthAzureCallback(config_obj *config_proto.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read oauthState from Cookie
		oauthState, _ := r.Cookie("oauthstate")

		if r.FormValue("state") != oauthState.Value {
			logging.GetLogger(config_obj, &logging.GUIComponent).
				Error("invalid oauth azure state")
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		user_info, err := getUserDataFromAzure(
			r.Context(), config_obj, r.FormValue("code"))
		if err != nil {
			logging.GetLogger(config_obj, &logging.GUIComponent).
				WithFields(logrus.Fields{
					"err": err,
				}).Error("getUserDataFromAzure")
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		// Create a new token object, specifying signing method and the claims
		// you would like it to contain.
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"user": user_info.Mail,

			// Require re-auth after one day.
			"expires": float64(time.Now().AddDate(0, 0, 1).Unix()),
			"picture": "/auth/azure/picture",
			"token":   user_info.Token,
		})

		// Sign and get the complete encoded token as a string using the secret
		tokenString, err := token.SignedString(
			[]byte(config_obj.Frontend.PrivateKey))
		if err != nil {
			logging.GetLogger(config_obj, &logging.GUIComponent).
				WithFields(logrus.Fields{
					"err": err,
				}).Error("getUserDataFromAzure")
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}

		// Set the cookie and redirect.
		cookie := &http.Cookie{
			Name:    "VelociraptorAuth",
			Value:   tokenString,
			Path:    "/",
			Secure:  true,
			Expires: time.Now().AddDate(0, 0, 1),
		}
		http.SetCookie(w, cookie)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	})
}

func getAzureOauthConfig(config_obj *config_proto.Config) *oauth2.Config {
	authenticator := config_obj.GUI.Authenticator
	return &oauth2.Config{
		RedirectURL:  config_obj.GUI.PublicUrl + "auth/azure/callback",
		ClientID:     authenticator.OauthClientId,
		ClientSecret: authenticator.OauthClientSecret,
		Scopes:       []string{"User.Read"},
		Endpoint:     microsoft.AzureADEndpoint(authenticator.Tenant),
	}
}

func getUserDataFromAzure(ctx context.Context,
	config_obj *config_proto.Config, code string) (*AzureUser, error) {

	// Use code to get token and get user info from Azure.
	azureOauthConfig := getAzureOauthConfig(config_obj)

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
func oauthAzurePicture(config_obj *config_proto.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		reject := func(err error) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
		}

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
		token_str, pres := claims["token"].(string)
		if !pres {
			reject(errors.New("token not present"))
			return
		}

		oauth_token := &oauth2.Token{}
		err = json.Unmarshal([]byte(token_str), &oauth_token)
		if err != nil {
			reject(err)
			return
		}

		azureOauthConfig := getAzureOauthConfig(config_obj)
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
