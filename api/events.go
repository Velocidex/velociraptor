package api

import (
	"crypto/x509"

	"github.com/golang/protobuf/ptypes/empty"
	context "golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"www.velocidex.com/golang/velociraptor/acls"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *ApiServer) PushEvents(
	ctx context.Context,
	in *api_proto.PushEventRequest) (*empty.Empty, error) {

	// Get the TLS context from the peer and verify its
	// certificate.
	peer, ok := peer.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "cant get peer info")
	}

	tlsInfo, ok := peer.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "unable to get credentials")
	}

	// Authenticate API clients using certificates.
	for _, peer_cert := range tlsInfo.State.PeerCertificates {
		chains, err := peer_cert.Verify(
			x509.VerifyOptions{Roots: self.ca_pool})
		if err != nil {
			return nil, err
		}

		if len(chains) == 0 {
			return nil, status.Error(codes.InvalidArgument, "no chains verified")
		}

		peer_name := crypto_utils.GetSubjectName(peer_cert)
		if peer_name != self.config.Client.PinnedServerName {
			token, err := acls.GetEffectivePolicy(self.config, peer_name)
			if err != nil {
				return nil, err
			}

			// Check that the principal is allowed to push to the queue.
			ok, err := acls.CheckAccessWithToken(token, acls.PUBLISH, in.Artifact)
			if err != nil {
				return nil, err
			}

			if !ok {
				return nil, status.Error(codes.PermissionDenied,
					"Permission denied: PUBLISH "+peer_name+" to "+in.Artifact)
			}
		}

		rows, err := utils.ParseJsonToDicts([]byte(in.Jsonl))
		if err != nil {
			return nil, err
		}

		// Only return the first row
		if true {
			journal, err := services.GetJournal()
			if err != nil {
				return nil, err
			}

			err = journal.PushRowsToArtifact(self.config,
				rows, in.Artifact, in.ClientId, in.FlowId)
			return &empty.Empty{}, err
		}
	}

	return nil, status.Error(codes.InvalidArgument, "no peer certs?")
}

func (self *ApiServer) WriteEvent(
	ctx context.Context,
	in *actions_proto.VQLResponse) (*empty.Empty, error) {

	// Get the TLS context from the peer and verify its
	// certificate.
	peer, ok := peer.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "cant get peer info")
	}

	tlsInfo, ok := peer.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "unable to get credentials")
	}

	// Authenticate API clients using certificates.
	for _, peer_cert := range tlsInfo.State.PeerCertificates {
		chains, err := peer_cert.Verify(
			x509.VerifyOptions{Roots: self.ca_pool})
		if err != nil {
			return nil, err
		}

		if len(chains) == 0 {
			return nil, status.Error(codes.InvalidArgument, "no chains verified")
		}

		peer_name := crypto_utils.GetSubjectName(peer_cert)

		token, err := acls.GetEffectivePolicy(self.config, peer_name)
		if err != nil {
			return nil, err
		}

		// Check that the principal is allowed to push to the queue.
		ok, err := acls.CheckAccessWithToken(token,
			acls.MACHINE_STATE, in.Query.Name)
		if err != nil {
			return nil, err
		}

		if !ok {
			return nil, status.Error(codes.PermissionDenied,
				"Permission denied: MACHINE_STATE "+
					peer_name+" to "+in.Query.Name)
		}

		rows, err := utils.ParseJsonToDicts([]byte(in.Response))
		if err != nil {
			return nil, err
		}

		// Only return the first row
		if true {
			journal, err := services.GetJournal()
			if err != nil {
				return nil, err
			}

			err = journal.PushRowsToArtifact(self.config,
				rows, in.Query.Name, peer_name, "")
			return &empty.Empty{}, err
		}
	}

	return nil, status.Error(codes.InvalidArgument, "no peer certs?")
}
