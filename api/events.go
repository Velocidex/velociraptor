package api

import (
	"os"
	"sort"
	"strings"

	context "golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"www.velocidex.com/golang/velociraptor/acls"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	file_store "www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *ApiServer) PushEvents(
	ctx context.Context,
	in *api_proto.PushEventRequest) (*emptypb.Empty, error) {

	defer Instrument("PushEvents")()

	users := services.GetUserManager()
	user_record, config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_record.Name
	token, err := acls.GetEffectivePolicy(config_obj, user_name)
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
			"Permission denied: PUBLISH "+user_name+" to "+in.Artifact)
	}

	rows, err := utils.ParseJsonToDicts([]byte(in.Jsonl))
	if err != nil {
		return nil, err
	}

	// The call can access the datastore from any org becuase it is a
	// server->server call.
	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil, err
	}

	org_config_obj, err := org_manager.GetOrgConfig(in.OrgId)
	if err != nil {
		return nil, err
	}

	// Only return the first row
	journal, err := services.GetJournal(org_config_obj)
	if err != nil {
		return nil, err
	}

	// only broadcast the events for local listeners. Minions
	// write the events themselves, so we just need to broadcast
	// for any server event artifacts that occur.
	journal.Broadcast(org_config_obj,
		rows, in.Artifact, in.ClientId, in.FlowId)
	return &emptypb.Empty{}, err
}

func (self *ApiServer) WriteEvent(
	ctx context.Context,
	in *actions_proto.VQLResponse) (*emptypb.Empty, error) {

	defer Instrument("WriteEvent")()

	users := services.GetUserManager()
	user_record, config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_record.Name
	token, err := acls.GetEffectivePolicy(config_obj, user_name)
	if err != nil {
		return nil, err
	}

	// Check that the principal is allowed to push to the queue.
	ok, err := acls.CheckAccessWithToken(token, acls.MACHINE_STATE, in.Query.Name)
	if err != nil {
		return nil, err
	}

	if !ok {
		return nil, status.Error(codes.PermissionDenied,
			"Permission denied: MACHINE_STATE "+
				user_name+" to "+in.Query.Name)
	}

	rows, err := utils.ParseJsonToDicts([]byte(in.Response))
	if err != nil {
		return nil, err
	}

	// The call can access the datastore from any org becuase it is a
	// server->server call.
	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil, err
	}

	org_config_obj, err := org_manager.GetOrgConfig(in.OrgId)
	if err != nil {
		return nil, err
	}

	// Only return the first row
	journal, err := services.GetJournal(org_config_obj)
	if err != nil {
		return nil, err
	}

	err = journal.PushRowsToArtifact(org_config_obj,
		rows, in.Query.Name, user_name, "")
	return &emptypb.Empty{}, err
}

func (self *ApiServer) ListAvailableEventResults(
	ctx context.Context,
	in *api_proto.ListAvailableEventResultsRequest) (
	*api_proto.ListAvailableEventResultsResponse, error) {

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(org_config_obj, user_record.Name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view results.")
	}

	if in.Artifact == "" {
		return listAvailableEventArtifacts(org_config_obj, self, in)
	}
	return listAvailableEventTimestamps(ctx, org_config_obj, self, in)
}

func listAvailableEventTimestamps(
	ctx context.Context,
	config_obj *config_proto.Config,
	self *ApiServer, in *api_proto.ListAvailableEventResultsRequest) (
	*api_proto.ListAvailableEventResultsResponse, error) {

	path_manager, err := artifacts.NewArtifactPathManager(
		config_obj, in.ClientId, "", in.Artifact)
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

	timestamps, err := listAvailableEventTimestampFiles(ctx, config_obj, self, path_manager)
	result.Logs[0].RowTimestamps = timestamps

	timestamps, err = listAvailableEventTimestampFiles(
		ctx, config_obj, self, path_manager.Logs())
	result.Logs[0].LogTimestamps = timestamps

	return result, nil
}

func listAvailableEventTimestampFiles(
	ctx context.Context,
	config_obj *config_proto.Config,
	self *ApiServer, path_manager api.PathManager) ([]int32, error) {
	result := []int32{}

	file_store_factory := file_store.GetFileStore(config_obj)
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
	config_obj *config_proto.Config,
	self *ApiServer,
	in *api_proto.ListAvailableEventResultsRequest) (
	*api_proto.ListAvailableEventResultsResponse, error) {

	// Figure out where all the monitoring artifacts logs are
	// stored by looking at some examples.
	exemplar := "Generic.Client.Stats"
	if in.ClientId == "" || in.ClientId == "server" {
		exemplar = "Server.Monitor.Health"
	}

	path_manager, err := artifacts.NewArtifactPathManager(
		config_obj, in.ClientId, "", exemplar)
	if err != nil {
		return nil, err
	}

	// getAllArtifacts analyses the path name from disk and adds
	// to the events list.
	seen := make(map[string]*api_proto.AvailableEvent)
	err = getAllArtifacts(config_obj, path_manager.GetRootPath(), seen)
	if err != nil {
		return nil, err
	}

	err = getAllArtifacts(config_obj, path_manager.Logs().GetRootPath(), seen)
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

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	return api.Walk(file_store_factory, log_path,
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
				if len(relative_path) == 0 {
					return nil
				}

				// Check if this is a valid artifact.
				artifact_base_name := relative_path[0]
				_, pres := repository.Get(config_obj, artifact_base_name)
				if !pres {
					return nil
				}

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
