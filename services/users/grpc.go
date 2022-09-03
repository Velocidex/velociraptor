package users

import (
	"context"
	"crypto/x509"
	"encoding/json"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self UserManager) GetUserFromContext(ctx context.Context) (
	*api_proto.VelociraptorUser, *config_proto.Config, error) {

	grpc_user_info := GetGRPCUserInfo(self.config_obj, ctx, self.ca_pool)
	user_record, err := self.GetUser(grpc_user_info.Name)
	if err != nil {
		return nil, nil, err
	}

	user_record.CurrentOrg = grpc_user_info.CurrentOrg

	// Fetch the appropriate config file fro the org manager.
	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil, nil, err
	}

	org_config_obj, err := org_manager.GetOrgConfig(user_record.CurrentOrg)
	return user_record, org_config_obj, err
}

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

						// Corresponds to the Grpc-Metadata-OrgId
						// header added by api-service.js
						org_id := md.Get("OrgId")
						if len(org_id) > 0 {
							result.CurrentOrg = org_id[0]
							if utils.IsRootOrg(result.CurrentOrg) {
								result.CurrentOrg = ""
							}
						}
					}
				}
			}
		}
	}

	return result
}
