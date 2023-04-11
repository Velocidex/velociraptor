package api

import (
	"time"

	"github.com/Velocidex/ordereddict"

	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/json"
	vjson "www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/hunt_dispatcher"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
)

func (self *ApiServer) GetHuntFlows(
	ctx context.Context,
	in *api_proto.GetTableRequest) (*api_proto.GetTableResponse, error) {

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.READ_RESULTS
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to view hunt results.")
	}

	hunt_dispatcher, err := services.GetHuntDispatcher(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	hunt, pres := hunt_dispatcher.GetHunt(in.HuntId)
	if !pres {
		return nil, status.Error(codes.InvalidArgument, "No hunt known")
	}

	total_scheduled := int64(-1)
	if hunt.Stats != nil {
		total_scheduled = int64(hunt.Stats.TotalClientsScheduled)
	}

	result := &api_proto.GetTableResponse{
		TotalRows: total_scheduled,
		Columns: []string{
			"ClientId", "Hostname", "FlowId", "StartedTime", "State", "Duration",
			"TotalBytes", "TotalRows",
		}}

	scope := vql_subsystem.MakeScope()
	for flow := range hunt_dispatcher.GetFlows(ctx, org_config_obj, scope,
		in.HuntId, int(in.StartRow)) {
		if flow.Context == nil {
			continue
		}

		row_data := []string{
			flow.Context.ClientId,
			services.GetHostname(ctx, org_config_obj, flow.Context.ClientId),
			flow.Context.SessionId,
			json.AnyToString(flow.Context.StartTime/1000, vjson.DefaultEncOpts()),
			flow.Context.State.String(),
			json.AnyToString(flow.Context.ExecutionDuration/1000000000,
				vjson.DefaultEncOpts()),
			json.AnyToString(flow.Context.TotalUploadedBytes, vjson.DefaultEncOpts()),
			json.AnyToString(flow.Context.TotalCollectedRows, vjson.DefaultEncOpts())}

		result.Rows = append(result.Rows, &api_proto.Row{Cell: row_data})

		if uint64(len(result.Rows)) > in.Rows {
			break
		}
	}
	return result, nil
}

func (self *ApiServer) CreateHunt(
	ctx context.Context,
	in *api_proto.Hunt) (*api_proto.StartFlowResponse, error) {

	defer Instrument("CreateHunt")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	// Log this event as an Audit event.
	in.Creator = principal
	in.HuntId = hunt_dispatcher.GetNewHuntId()

	acl_manager := acl_managers.NewServerACLManager(org_config_obj, in.Creator)

	// It is possible to start a paused hunt with the COLLECT_CLIENT
	// permission.
	permissions := acls.COLLECT_CLIENT

	// To actually start the hunt we need the START_HUNT
	// permission. This allows for division of responsibility between
	// hunt proposers and hunt starters.
	if in.State == api_proto.Hunt_RUNNING {
		permissions = acls.START_HUNT
	}

	perm, err := services.CheckAccess(org_config_obj, in.Creator, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to launch hunts.")
	}

	// Require the Org Admin permission to launch hunts in a differen
	// org.
	orgs := in.OrgIds
	if len(orgs) > 0 {
		permissions := acls.ORG_ADMIN
		perm, err := services.CheckAccess(org_config_obj, in.Creator, permissions)
		if !perm || err != nil {
			return nil, PermissionDenied(err,
				"User is not allowed to launch hunts in other orgs.")
		}
	} else {
		orgs = append(orgs, org_config_obj.OrgId)
	}

	logger := logging.GetLogger(org_config_obj, &logging.FrontendComponent)
	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	var orgs_we_scheduled []string

	for _, org_id := range orgs {
		org_config_obj, err := org_manager.GetOrgConfig(org_id)
		if err != nil {
			logger.Error("CreateHunt: GetOrgConfig %v", err)
			continue
		}

		// Make sure the user is allowed to collect in that org
		perm, err := services.CheckAccess(
			org_config_obj, in.Creator, permissions)
		if !perm {
			if err != nil {
				logger.Error("%v: CreateHunt: User is not allowed to launch hunts in "+
					"org %v.", err, org_id)
			}
			logger.Error("CreateHunt: User is not allowed to launch hunts in "+
				"org %v.", org_id)
			continue
		}

		hunt_dispatcher, err := services.GetHuntDispatcher(org_config_obj)
		if err != nil {
			logger.Error("CreateHunt: GetOrgConfig %v", err)
			continue
		}

		hunt_id, err := hunt_dispatcher.CreateHunt(
			ctx, org_config_obj, acl_manager, in)
		if err != nil {
			logger.Error("CreateHunt: GetOrgConfig %v", err)
			continue
		}

		orgs_we_scheduled = append(orgs_we_scheduled, org_id)
		// Reuse the hunt id for all the hunts we launch on all the
		// orgs - this makes it easier to combine results from all
		// orgs.
		in.HuntId = hunt_id
	}

	result := &api_proto.StartFlowResponse{}
	result.FlowId = in.HuntId

	// Audit message for GUI access
	logging.LogAudit(org_config_obj, principal, "CreateHunt",
		logrus.Fields{
			"hunt_id": result.FlowId,
			"details": json.MustMarshalString(in),
			"orgs":    orgs_we_scheduled,
		})

	return result, nil
}

func (self *ApiServer) ModifyHunt(
	ctx context.Context,
	in *api_proto.Hunt) (*emptypb.Empty, error) {

	defer Instrument("ModifyHunt")()

	// Log this event as an Audit event.
	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	in.Creator = principal

	permissions := acls.COLLECT_CLIENT
	if in.State == api_proto.Hunt_RUNNING {
		permissions = acls.START_HUNT
	}

	perm, err := services.CheckAccess(org_config_obj, in.Creator, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to modify hunts.")
	}

	logging.LogAudit(org_config_obj, principal, "ModifyHunt",
		logrus.Fields{
			"hunt_id": in.HuntId,
			"details": json.MustMarshalString(in),
		})

	hunt_dispatcher, err := services.GetHuntDispatcher(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	err = hunt_dispatcher.ModifyHunt(ctx, org_config_obj, in, in.Creator)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	result := &emptypb.Empty{}
	return result, nil
}

func (self *ApiServer) ListHunts(
	ctx context.Context,
	in *api_proto.ListHuntsRequest) (*api_proto.ListHuntsResponse, error) {

	defer Instrument("ListHunts")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.READ_RESULTS
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to view hunts.")
	}

	hunt_dispatcher, err := services.GetHuntDispatcher(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	result, err := hunt_dispatcher.ListHunts(
		ctx, org_config_obj, in)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	// Provide only a summary for list hunts GUI
	if in.Summary {
		summary := &api_proto.ListHuntsResponse{}
		for _, item := range result.Items {
			summary.Items = append(summary.Items, &api_proto.Hunt{
				HuntId:          item.HuntId,
				HuntDescription: item.HuntDescription,
				State:           item.State,
				Creator:         item.Creator,
				CreateTime:      item.CreateTime,
				StartTime:       item.StartTime,
				Stats:           item.Stats,
				Expires:         item.Expires,
			})
		}

		return summary, nil
	}

	return result, nil
}

func (self *ApiServer) GetHunt(
	ctx context.Context,
	in *api_proto.GetHuntRequest) (*api_proto.Hunt, error) {
	if in.HuntId == "" {
		return &api_proto.Hunt{}, nil
	}

	defer Instrument("GetHunt")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.READ_RESULTS
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to view hunts.")
	}

	hunt_dispatcher, err := services.GetHuntDispatcher(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	result, pres := hunt_dispatcher.GetHunt(in.HuntId)
	if !pres {
		return nil, InvalidStatus("Hunt not found")
	}

	return result, nil
}

func (self *ApiServer) GetHuntResults(
	ctx context.Context,
	in *api_proto.GetHuntResultsRequest) (*api_proto.GetTableResponse, error) {

	defer Instrument("GetHuntResults")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.READ_RESULTS
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to view results.")
	}

	env := ordereddict.NewDict().
		Set("HuntID", in.HuntId).
		Set("ArtifactName", in.Artifact)

	// More than 100 results are not very useful in the GUI -
	// users should just download the json file for post
	// processing or process in the notebook.
	result, err := RunVQL(ctx, org_config_obj, principal, env,
		"SELECT * FROM hunt_results(hunt_id=HuntID, "+
			"artifact=ArtifactName) LIMIT 100")
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	return result, nil
}

func (self *ApiServer) EstimateHunt(
	ctx context.Context,
	in *api_proto.HuntEstimateRequest) (*api_proto.HuntStats, error) {

	defer Instrument("EstimateHunt")()
	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.READ_RESULTS
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to view hunt results.")
	}

	client_info_manager, err := services.GetClientInfoManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	indexer, err := services.GetIndexer(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	now := uint64(time.Now().UnixNano() / 1000)

	is_client_recent := func(client_id string, seen map[string]bool) {
		// We dont care about last active status
		if in.LastActive == 0 {
			seen[client_id] = true
			return
		}

		stats, err := client_info_manager.GetStats(ctx, client_id)
		if err == nil && now-in.LastActive*1000000 < stats.Ping {
			seen[client_id] = true
		}
	}

	if in.Condition != nil {
		labels := in.Condition.GetLabels()
		if labels != nil && len(labels.Label) > 0 {
			// Multiple labels imply an OR relationship - if a client
			// has any of the labels set, it will be scheduled.
			seen := make(map[string]bool)
			for _, label := range labels.Label {
				for entity := range indexer.SearchIndexWithPrefix(
					ctx, org_config_obj, "label:"+label) {
					is_client_recent(entity.Entity, seen)
				}
			}

			// Remove any excluded labels.
			if in.Condition.ExcludedLabels != nil {
				for _, label := range in.Condition.ExcludedLabels.Label {
					for entity := range indexer.SearchIndexWithPrefix(
						ctx, org_config_obj, "label:"+label) {
						delete(seen, entity.Entity)
					}
				}
			}

			return &api_proto.HuntStats{
				TotalClientsScheduled: uint64(len(seen)),
			}, nil
		}

		os_condition := in.Condition.GetOs()
		if os_condition != nil &&
			os_condition.Os != api_proto.HuntOsCondition_ALL {
			seen := make(map[string]bool)
			os_name := ""
			switch os_condition.Os {
			case api_proto.HuntOsCondition_WINDOWS:
				os_name = "windows"
			case api_proto.HuntOsCondition_LINUX:
				os_name = "linux"
			case api_proto.HuntOsCondition_OSX:
				os_name = "darwin"
			}

			client_info_manager, err := services.GetClientInfoManager(org_config_obj)
			if err != nil {
				return nil, Status(self.verbose, err)
			}

			for hit := range indexer.SearchIndexWithPrefix(ctx,
				org_config_obj, "all") {
				client_id := hit.Entity
				client_info, err := client_info_manager.Get(ctx, client_id)
				if err == nil {
					if os_name == client_info.System {
						is_client_recent(hit.Entity, seen)
					}
				}
			}

			// Remove any excluded labels.
			if in.Condition.ExcludedLabels != nil {
				for _, label := range in.Condition.ExcludedLabels.Label {
					for entity := range indexer.SearchIndexWithPrefix(
						ctx, org_config_obj, "label:"+label) {
						delete(seen, entity.Entity)
					}
				}
			}

			return &api_proto.HuntStats{
				TotalClientsScheduled: uint64(len(seen)),
			}, nil
		}

		// No condition, just count all the clients.
		seen := make(map[string]bool)
		for hit := range indexer.SearchIndexWithPrefix(ctx, org_config_obj, "all") {
			is_client_recent(hit.Entity, seen)
		}

		// Remove any excluded labels.
		if in.Condition.ExcludedLabels != nil {
			for _, label := range in.Condition.ExcludedLabels.Label {
				for entity := range indexer.SearchIndexWithPrefix(
					ctx, org_config_obj, "label:"+label) {
					delete(seen, entity.Entity)
				}
			}
		}

		return &api_proto.HuntStats{
			TotalClientsScheduled: uint64(len(seen)),
		}, nil
	}

	// No condition, just count all the clients.
	seen := make(map[string]bool)
	for hit := range indexer.SearchIndexWithPrefix(ctx, org_config_obj, "all") {
		is_client_recent(hit.Entity, seen)
	}

	return &api_proto.HuntStats{
		TotalClientsScheduled: uint64(len(seen)),
	}, nil
}
