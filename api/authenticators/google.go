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
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	api_utils "www.velocidex.com/golang/velociraptor/api/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

type GoogleOidcRouter struct {
	config_obj *config_proto.Config
}

func (self *GoogleOidcRouter) Name() string {
	return "Google"
}

func (self *GoogleOidcRouter) LoginHandler() string {
	return "/auth/google/login"
}

func (self *GoogleOidcRouter) CallbackHandler() string {
	return "/auth/google/callback"
}

func (self *GoogleOidcRouter) Scopes() []string {
	return []string{"https://www.googleapis.com/auth/userinfo.email"}
}

func (self *GoogleOidcRouter) Issuer() string {
	return "https://accounts.google.com"
}

func (self *GoogleOidcRouter) Endpoint() oauth2.Endpoint {
	return google.Endpoint
}

func (self *GoogleOidcRouter) SetEndpoint(oauth2.Endpoint) {}

func (self *GoogleOidcRouter) Avatar() string {
	return ""
}

func (self *GoogleOidcRouter) LoginURL() string {
	return api_utils.PublicURL(self.config_obj, self.LoginHandler())
}
