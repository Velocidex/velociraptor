package api

import (
	"crypto/x509"
	"os"
	"sort"
	"strings"

	"github.com/golang/protobuf/ptypes/empty"
	context "golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"www.velocidex.com/golang/velociraptor/acls"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	file_store "www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	users "www.velocidex.com/golang/velociraptor/users"
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

func (self *ApiServer) ListAvailableEventResults(
	ctx context.Context,
	in *api_proto.ListAvailableEventResultsRequest) (
	*api_proto.ListAvailableEventResultsResponse, error) {

	user_name := GetGRPCUserInfo(self.config, ctx, self.ca_pool).Name
	user_record, err := users.GetUser(self.config, user_name)
	if err != nil {
		return nil, err
	}

	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_record.Name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view results.")
	}

	if in.Artifact == "" {
		return listAvailableEventArtifacts(self, in)
	}
	return listAvailableEventTimestamps(ctx, self, in)
}

func listAvailableEventTimestamps(
	ctx context.Context,
	self *ApiServer, in *api_proto.ListAvailableEventResultsRequest) (
	*api_proto.ListAvailableEventResultsResponse, error) {

	path_manager, err := artifacts.NewArtifactPathManager(
		self.config, in.ClientId, "", in.Artifact)
	if err != nil {
		return nil, err
	}

	result := &api_proto.ListAvailableEventResultsResponse{
		Logs: []*api_proto.AvailableEvent{
			{
				Artifact: in.Artifact,
			},
		},
	}

	timestamps, err := listAvailableEventTimestampFiles(ctx, self, path_manager)
	result.Logs[0].RowTimestamps = timestamps

	timestamps, err = listAvailableEventTimestampFiles(
		ctx, self, path_manager.Logs())
	result.Logs[0].LogTimestamps = timestamps

	return result, nil
}

func listAvailableEventTimestampFiles(
	ctx context.Context, self *ApiServer, path_manager api.PathManager) ([]int32, error) {
	result := []int32{}

	file_store_factory := file_store.GetFileStore(self.config)
	reader, err := result_sets.NewTimedResultSetReader(
		ctx, file_store_factory, path_manager)
	if err != nil {
		return nil, err
	}

	for _, prop := range reader.GetAvailableFiles(ctx) {
		result = append(result, int32(prop.StartTime.Unix()))
	}
	return result, nil
}

func listAvailableEventArtifacts(
	self *ApiServer, in *api_proto.ListAvailableEventResultsRequest) (
	*api_proto.ListAvailableEventResultsResponse, error) {

	// Figure out where all the monitoring artifacts logs are
	// stored by looking at some examples.
	exemplar := "Generic.Client.Stats"
	if in.ClientId == "" || in.ClientId == "server" {
		exemplar = "Server.Monitor.Health"
	}

	path_manager, err := artifacts.NewArtifactPathManager(
		self.config, in.ClientId, "", exemplar)
	if err != nil {
		return nil, err
	}

	// getAllArtifacts analyses the path name from disk and adds
	// to the events list.
	seen := make(map[string]*api_proto.AvailableEvent)
	err = getAllArtifacts(self.config, path_manager.GetRootPath(), seen)
	if err != nil {
		return nil, err
	}

	err = getAllArtifacts(self.config, path_manager.Logs().GetRootPath(), seen)
	if err != nil {
		return nil, err
	}

	result := &api_proto.ListAvailableEventResultsResponse{}
	for _, item := range seen {
		result.Logs = append(result.Logs, item)
	}

	sort.Slice(result.Logs, func(i, j int) bool {
		return result.Logs[i].Artifact < result.Logs[j].Artifact
	})

	return result, nil
}

func getAllArtifacts(
	config_obj *config_proto.Config,
	log_path api.FSPathSpec,
	seen map[string]*api_proto.AvailableEvent) error {

	file_store_factory := file_store.GetFileStore(config_obj)

	return file_store_factory.Walk(log_path,
		func(full_path api.FSPathSpec, info os.FileInfo) error {
			// Walking the events directory will give us
			// all the day json files. Each day json file
			// is contained in a directory structure which
			// reflects the name of the artifact, for
			// example:

			// <log_path>/Server.Monitor.Health/Prometheus/2021-08-01.json
			// Corresponds to the artifact Server.Monitor.Health/Prometheus
			if !info.IsDir() && info.Size() > 0 {
				relative_path := full_path.Dir().
					Components()[len(log_path.Components()):]
				artifact_name := strings.Join(relative_path, "/")
				event, pres := seen[artifact_name]
				if !pres {
					event = &api_proto.AvailableEvent{
						Artifact: artifact_name,
					}
					seen[artifact_name] = event
				}
			}
			return nil
		})
}
