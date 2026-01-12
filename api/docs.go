package api

import (
	"context"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

/*

# How does the Velociraptor server work?

The Velociraptor server presents a GRPC API for manipulating and
presenting data. We chose gRPC because:

1. It has mutual two way authentication - the server presents a
   certificate to identify itself and the caller must also present a
   properly signed certificate.

2. Communication is encrypted using TLS

3. Each RPC call can include a complex well defined API with protocol
   buffers as inputs and outputs.

4. GRPC has a streaming mode which is useful for real time
   communications (e.g. via the Query API point).

The server's API surface is well defined in api/proto/api.proto and
implemented in this "api" module.

By tightening down the server api surface it is easier to ensure that
ACLs are properly enforced.

## ACLs and permissions

The API endpoints enforce the permission model based on the identity
of the caller. In the gRPC API the caller's identity is found by
examining the Common Name in the certificate that the user presented.

The user identity is recovered using the users service
users.GetUserFromContext(ctx)

## How is the GUI implemented?

The GUI is a simple react app which communicates with the server using
AJAX calls, such as GET or POST. As such the GUI can not make direct
gRPC calls to the API server.

To translate between REST calls to gRPC we use the grpc gateway
proxy. This proxy service exposes HTTP handlers on /api/ URLs. When a
HTTP connection occurs, the gateway proxy will bundle the data into
protocol buffers and make a proper gRPC call into the API.

The gateway's gRPC connections are made using the gateway identity
(certificates generated in GUI.gw_certificate and
GUI.gw_private_key. The real identity of the calling user is injected
in the gRPC metadata channel under the "USER" parameter.

From the API server's perspective, the user identity is:

1. If the identity is not utils.GetGatewayName() then the identity is
   fetched from the caller's X509 certificates. (After verifying the
   certificates are issued by the internal CA)

2. If the caller is really the Gateway, then the real identity of the
   user is retrieved from the gRPC metadata "USER" variable (passed in
   the context)

This logic is implemented in services/users/grpc.go:GetGRPCUserInfo()

NOTE: The gateway's certificates are critical to protect - if an actor
  makes an API connection using these certificate they can just claim
  to be anyone by injecting any username into the "USER" gRPC
  metadata.


*/

func (self *ApiServer) SearchDocs(
	ctx context.Context,
	in *api_proto.DocSearchRequest) (*api_proto.DocSearchResponses, error) {

	users := services.GetUserManager()
	_, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	// All users can search the docs with no permission required.
	doc_manager, err := services.GetDocManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	return doc_manager.Search(ctx, in.Query, int(in.Start), int(in.Length))
}
