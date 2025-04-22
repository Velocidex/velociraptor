package api

import (
	"context"

	errors "github.com/go-errors/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"www.velocidex.com/golang/velociraptor/acls"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/json"
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
		return nil, Status(self.verbose, err)
	}

	// User is asking to switch orgs
	if in.OrgId != "" {
		org_manager, err := services.GetOrgManager()
		if err != nil {
			return nil, Status(self.verbose, err)
		}

		org_config_obj, err = org_manager.GetOrgConfig(in.OrgId)
		if err != nil {
			return nil, Status(self.verbose, err)
		}
	}

	user_name := user_record.Name

	// Now check permmissions in the org if the user is not the superuser.
	if user_name != utils.GetSuperuserName(org_config_obj) {
		token, err := services.GetEffectivePolicy(org_config_obj, user_name)
		if err != nil {
			return nil, Status(self.verbose, err)
		}

		// Check that the principal is allowed to push to this specific queue.
		ok, err := services.CheckAccessWithToken(token, acls.PUBLISH, in.Artifact)
		if err != nil {
			return nil, Status(self.verbose, err)
		}

		if !ok {
			return nil, status.Error(codes.PermissionDenied,
				"Permission denied: PUBLISH "+user_name+" to "+in.Artifact)
		}

		// For regular users append the sender field so we can track
		// where the message came from.
		in.Jsonl = json.AppendJsonlItem(in.Jsonl, "_Sender", user_name)

		// Always write user events.
		in.Write = true
	}

	rows, err := utils.ParseJsonToDicts([]byte(in.Jsonl))
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	// Only return the first row
	journal, err := services.GetJournal(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	// only broadcast the events for local listeners. Minions
	// write the events themselves, so we just need to broadcast
	// for any server event artifacts that occur.
	if in.Write {
		err = journal.PushRowsToArtifact(ctx, org_config_obj,
			rows, in.Artifact, in.ClientId, in.FlowId)

	} else {
		err = journal.Broadcast(ctx, org_config_obj,
			rows, in.Artifact, in.ClientId, in.FlowId)

	}

	return &emptypb.Empty{}, err
}

func (self *ApiServer) WriteEvent(
	ctx context.Context,
	in *actions_proto.VQLResponse) (*emptypb.Empty, error) {
	return nil, Status(self.verbose,
		errors.New("WriteEvent is deprecated, please use PushEvents instead"))
}

func (self *ApiServer) ListAvailableEventResults(
	ctx context.Context,
	in *api_proto.ListAvailableEventResultsRequest) (
	*api_proto.ListAvailableEventResultsResponse, error) {

	defer Instrument("ListAvailableEventResults")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	permissions := acls.READ_RESULTS
	perm, err := services.CheckAccess(org_config_obj, user_record.Name, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to view results.")
	}

	client_monitoring_service, err := services.ClientEventManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	return client_monitoring_service.ListAvailableEventResults(ctx, in)
}
