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
package api

import (
	"context"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
)

// GetGRPCUserInfo: Extracts user information from GRPC context.
func GetGRPCUserInfo(
	config_obj *config_proto.Config,
	ctx context.Context) *api_proto.VelociraptorUser {
	result := &api_proto.VelociraptorUser{}

	peer, ok := peer.FromContext(ctx)
	if ok {
		tlsInfo, ok := peer.AuthInfo.(credentials.TLSInfo)
		if ok && config_obj.API != nil {
			v := tlsInfo.State.PeerCertificates[0].Subject.CommonName

			// Calls from the gRPC gateway are allowed to
			// embed the authenticated web user in the
			// metadata. This allows the API gateway to
			// impersonate anyone - it must be trusted to
			// convert web side authentication to a valid
			// user name which it may pass in the call
			// context.
			if v == config_obj.API.PinnedGwName {
				userinfo, ok := ctx.Value(constants.GRPC_USER_CONTEXT).(string)
				if ok {
					data := []byte(userinfo)
					err := json.Unmarshal(data, result)
					if err != nil {
						logging.GetLogger(config_obj, &logging.GUIComponent).Error(
							"Unable to Unmarshal USER Token")
						result.Name = ""
					}
				}

				/*
					md, ok := metadata.FromIncomingContext(ctx)
					if ok {
						userinfo := md.Get(constants.GRPC_USER_CONTEXT)
						if len(userinfo) > 0 {
							data := []byte(userinfo[0])
							err := json.Unmarshal(data, result)
							if err != nil {
								logger := logging.GetLogger(config_obj,
									&logging.FrontendComponent)
								logger.Error("GetGRPCUserInfo: %v", err)
								result.Name = ""
							}
						}
					}
				*/
			}

			// Other callers will return the name on their
			// certificate.
			if result.Name == "" {
				result.Name = v
			}
		}
	}

	return result
}

func NewDefaultUserObject(config_obj *config_proto.Config) *api_proto.ApiGrrUser {
	result := &api_proto.ApiGrrUser{
		UserType: api_proto.ApiGrrUser_USER_TYPE_ADMIN,
	}

	if config_obj.GUI != nil {
		result.InterfaceTraits = &api_proto.ApiGrrUserInterfaceTraits{
			AuthUsingGoogle: config_obj.GUI.GoogleOauthClientId != "",
			Links:           []*api_proto.UILink{},
		}

		for _, link := range config_obj.GUI.Links {
			result.InterfaceTraits.Links = append(result.InterfaceTraits.Links,
				&api_proto.UILink{Text: link.Text, Url: link.Url})
		}
	}

	return result
}
