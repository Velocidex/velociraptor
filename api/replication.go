package api

import (
	"crypto/x509"
	"fmt"

	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
)

func streamEvents(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.EventRequest,
	stream api_proto.API_WatchEventServer,
	peer_name string) (err error) {

	logger := logging.GetLogger(config_obj, &logging.APICmponent)
	logger.WithFields(logrus.Fields{
		"arg":  in,
		"user": peer_name,
	}).Info("Replicating Events")

	journal, err := services.GetJournal()
	if err != nil {
		return err
	}

	// The API service is running on the master only! This means
	// the journal service is local.
	output_chan, cancel := journal.Watch(ctx, in.Queue)
	defer cancel()

	for event := range output_chan {
		serialized, err := json.Marshal(event)
		if err != nil {
			continue
		}
		response := &api_proto.EventResponse{
			Jsonl: serialized,
		}
		err = stream.Send(response)
		if err != nil {
			continue
		}
	}

	return nil
}

// NOTE: The API server is only running on the master node.
func (self *ApiServer) WatchEvent(
	in *api_proto.EventRequest,
	stream api_proto.API_WatchEventServer) error {

	// Get the TLS context from the peer and verify its
	// certificate.
	ctx := stream.Context()
	peer, ok := peer.FromContext(ctx)
	if !ok {
		return status.Error(codes.InvalidArgument, "cant get peer info")
	}

	tlsInfo, ok := peer.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return status.Error(codes.InvalidArgument, "unable to get credentials")
	}

	// Authenticate API clients using certificates.
	for _, peer_cert := range tlsInfo.State.PeerCertificates {
		chains, err := peer_cert.Verify(
			x509.VerifyOptions{Roots: self.ca_pool})
		if err != nil {
			return err
		}

		if len(chains) == 0 {
			return status.Error(codes.InvalidArgument, "no chains verified")
		}

		peer_name := crypto.GetSubjectName(peer_cert)

		// Check that the principal is allowed to issue queries.
		permissions := acls.ANY_QUERY
		ok, err := acls.CheckAccess(self.config, peer_name, permissions)
		if err != nil {
			return status.Error(codes.PermissionDenied,
				fmt.Sprintf("User %v is not allowed to run queries.",
					peer_name))
		}

		if !ok {
			return status.Error(codes.PermissionDenied, fmt.Sprintf(
				"Permission denied: User %v requires permission %v to run queries",
				peer_name, permissions))
		}

		// return the first good match
		if true {
			// Cert is good enough for us, run the query.
			return streamEvents(ctx, self.config, in, stream, peer_name)
		}
	}

	return status.Error(codes.InvalidArgument, "no peer certs?")
}
