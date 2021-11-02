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
	"crypto/x509"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
)

// GetGRPCUserInfo: Extracts user information from GRPC context.
func GetGRPCUserInfo(
	config_obj *config_proto.Config,
	ctx context.Context,
	ca_pool *x509.CertPool) *api_proto.VelociraptorUser {
	result := &api_proto.VelociraptorUser{}

	// Check for remote TLS client certs.
	peer, ok := peer.FromContext(ctx)
	if ok {
		tlsInfo, ok := peer.AuthInfo.(credentials.TLSInfo)
		if ok && config_obj.API != nil {
			// Extract the name from each incoming certificate
			for _, peer_cert := range tlsInfo.State.PeerCertificates {

				// This certificate is not valid, skip it.
				chains, err := peer_cert.Verify(
					x509.VerifyOptions{Roots: ca_pool})
				if err != nil || len(chains) == 0 {
					continue
				}

				result.Name = crypto_utils.GetSubjectName(
					tlsInfo.State.PeerCertificates[0])

				// Calls from the gRPC gateway are allowed to
				// embed the authenticated web user in the
				// metadata. This allows the API gateway to
				// impersonate anyone - it must be trusted to
				// convert web side authentication to a valid
				// user name which it may pass in the call
				// context.
				if result.Name == config_obj.API.PinnedGwName {
					md, ok := metadata.FromIncomingContext(ctx)
					if ok {
						userinfo := md.Get("USER")
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
				}
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
