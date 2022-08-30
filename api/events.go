package api

import (
	context "golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"www.velocidex.com/golang/velociraptor/acls"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *ApiServer) PushEvents(
	ctx context.Context,
	in *api_proto.PushEventRequest) (*emptypb.Empty, error) {

	defer Instrument("PushEvents")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_record.Name
	token, err := acls.GetEffectivePolicy(org_config_obj, user_name)
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
	if token.SuperUser && org_config_obj.OrgId != in.OrgId {
		org_manager, err := services.GetOrgManager()
		if err != nil {
			return nil, err
		}

		org_config_obj, err = org_manager.GetOrgConfig(in.OrgId)
		if err != nil {
			return nil, err
		}
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

	client_monitoring_service, err := services.ClientEventManager(org_config_obj)
	if err != nil {
		return nil, err
	}

	return client_monitoring_service.ListAvailableEventResults(ctx, in)
}
