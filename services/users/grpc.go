package users

import (
	"context"
	"crypto/x509"
	"errors"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self UserManager) GetUserFromContext(ctx context.Context) (
	user_record *api_proto.VelociraptorUser, org_config_obj *config_proto.Config, err error) {

	grpc_user_info := GetGRPCUserInfo(self.config_obj, ctx, self.ca_pool)

	// If the call comes from the super user we allow it.
	if grpc_user_info.Name == utils.GetSuperuserName(org_config_obj) {
		user_record = &api_proto.VelociraptorUser{
			Name:            grpc_user_info.Name,
			UserIsSuperuser: true,
		}

	} else {
		user_record, err = self.storage.GetUserWithHashes(ctx, grpc_user_info.Name)
		if err != nil {
			return nil, nil, err
		}

		self.normalizeOrgList(ctx, user_record)
	}

	user_record.CurrentOrg = grpc_user_info.CurrentOrg
	user_record.PasswordSalt = nil
	user_record.PasswordHash = nil

	// Fetch the appropriate config file from the org manager.
	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil, nil, err
	}

	org_config_obj, err = org_manager.GetOrgConfig(user_record.CurrentOrg)
	user_record.UserIsSuperuser = false
	return user_record, org_config_obj, err
}

// Used for direct HTTP connections. User context was added by the
// authenticator.
func (self UserManager) GetUserFromHTTPContext(ctx context.Context) (
	user_record *api_proto.VelociraptorUser, err error) {

	user_info := &api_proto.VelociraptorUser{}
	serialized_any := ctx.Value(constants.GRPC_USER_CONTEXT)
	if serialized_any == nil {
		return nil, errors.New("Unauthenticated request")
	}

	serialized, ok := serialized_any.(string)
	if !ok {
		return nil, errors.New("Unauthenticated request")
	}
	user_info.Name = utils.UnQuote(serialized)
	return user_info, err
}

// Figure out if the username can impersonate another user in this RPC
// call. Impersonating users are making an RPC call on behalf of other
// users:
//  1. The GRPC gateway carries the HTTP auth from a user to the RPC
//     endpoint.
//  2. Redirected orgs carry the originating user identity to the
//     remote server.
func canUserImpersonate(
	config_obj *config_proto.Config,
	username string) bool {
	// The gateway can always impersonate
	if username == utils.GetGatewayName(config_obj) {
		return true
	}

	// The user must have all these permissions. This is used for
	// remote datastore connections such as org redirection.
	ok, err := services.CheckAccess(config_obj, username,
		acls.IMPERSONATION,
		acls.ORG_ADMIN,
		// Must have API level access
		acls.API_QUERY)
	return err == nil && ok
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

				// Calls from the gRPC gateway are allowed to embed
				// the authenticated web user in the metadata. This
				// allows the API gateway to impersonate anyone - it
				// must be trusted to convert web side authentication
				// to a valid user name which it may pass in the call
				// context.
				if canUserImpersonate(config_obj, result.Name) {
					md, ok := metadata.FromIncomingContext(ctx)
					if ok {
						userinfo := md.Get(constants.GRPC_MD_USER)
						if len(userinfo) == 1 {
							// gRPC metadata can only contain ASCII so
							// we recover the UTF8 string from there.
							result = &api_proto.VelociraptorUser{
								Name: utils.UnQuote(userinfo[0]),
							}
						}

						// Corresponds to the Grpc-Metadata-OrgId
						// header added by api-service.js
						org_id := md.Get(constants.GRPC_MD_ORG_ID)
						if len(org_id) == 1 {
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
