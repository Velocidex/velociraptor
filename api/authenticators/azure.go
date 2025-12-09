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
	"encoding/base64"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/microsoft"
	api_utils "www.velocidex.com/golang/velociraptor/api/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	utils "www.velocidex.com/golang/velociraptor/utils"
)

const (
	AzureIcon = "data:image/png;base64,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"
)

type AzureUser struct {
	Mail    string `json:"userPrincipalName"`
	Name    string `json:"displayName"`
	Picture string `json:"picture"`
}

type AzureOidcRouter struct {
	config_obj    *config_proto.Config
	authenticator *config_proto.Authenticator
}

func (self *AzureOidcRouter) Name() string {
	return "Azure"
}

func (self *AzureOidcRouter) LoginHandler() string {
	return "/auth/azure/login"
}

func (self *AzureOidcRouter) CallbackHandler() string {
	return "/auth/azure/callback"
}

func (self *AzureOidcRouter) Scopes() []string {
	return []string{"User.Read"}
}

func (self *AzureOidcRouter) Issuer() string {
	return ""
}

func (self *AzureOidcRouter) Endpoint() oauth2.Endpoint {
	return microsoft.AzureADEndpoint(self.authenticator.Tenant)
}

func (self *AzureOidcRouter) SetEndpoint(oauth2.Endpoint) {}

func (self *AzureOidcRouter) Avatar() string {
	return AzureIcon
}

func (self *AzureOidcRouter) LoginURL() string {
	return api_utils.PublicURL(self.config_obj, self.LoginHandler())
}

type AzureClaimsGetter struct {
	config_obj *config_proto.Config
	router     OidcRouter
}

func (self *AzureClaimsGetter) GetClaims(
	ctx *HTTPClientContext, token *oauth2.Token) (claims *Claims, err error) {

	oauthConfig := &oauth2.Config{
		Endpoint: self.router.Endpoint(),
	}

	client := oauthConfig.Client(ctx, token)
	response, err := client.Get("https://graph.microsoft.com/v1.0/me/")
	if err != nil {
		return nil, fmt.Errorf("failed getting user info: %v", err)
	}
	defer response.Body.Close()

	contents, err := utils.ReadAllWithLimit(response.Body, constants.MAX_MEMORY)
	if err != nil {
		return nil, fmt.Errorf("failed read response: %v", err)
	}

	user_info := &AzureUser{}
	err = json.Unmarshal(contents, &user_info)
	if err != nil {
		return nil, err
	}

	username := user_info.Mail
	if username != "" {
		picture := self.getAzurePicture(client)
		if picture != "" {
			setUserPicture(ctx, username, picture)
		}
	}

	return &Claims{
		Username: user_info.Mail,
	}, nil
}

// Best effort - if anything fails we just dont show the picture.
func (self *AzureClaimsGetter) getAzurePicture(client *http.Client) string {
	response, err := client.Get("https://graph.microsoft.com/v1.0/me/photos/48x48/$value")
	if err != nil {
		return ""
	}
	defer response.Body.Close()

	data, _ := utils.ReadAllWithLimit(response.Body,
		constants.MAX_MEMORY)

	return fmt.Sprintf("data:image/jpeg;base64,%v",
		base64.StdEncoding.EncodeToString(data))
}
