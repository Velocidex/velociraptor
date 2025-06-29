package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	errors "github.com/go-errors/errors"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/api/tables"
	"www.velocidex.com/golang/velociraptor/json"
	vjson "www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/hunt_dispatcher"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
)

func (self *ApiServer) GetHuntFlows(
	ctx context.Context,
	in *api_proto.GetTableRequest) (*api_proto.GetTableResponse, error) {

	defer Instrument("GetHuntFlows")()

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

	options, err := tables.GetTableOptions(in)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	hunt_dispatcher, err := services.GetHuntDispatcher(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	scope := vql_subsystem.MakeScope()
	flow_chan, total_rows, err := hunt_dispatcher.GetFlows(
		ctx, org_config_obj,
		services.FlowSearchOptions{
			ResultSetOptions: options,
		},
		scope, in.HuntId, int(in.StartRow))
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	result := &api_proto.GetTableResponse{
		TotalRows: total_rows,
		Columns: []string{
			"ClientId", "Hostname", "FlowId", "StartedTime", "State", "Duration",
			"TotalBytes", "TotalRows",
		}}

	for flow := range flow_chan {
		if flow.Context == nil {
			continue
		}

		row_data := []interface{}{
			flow.Context.ClientId,
			services.GetHostname(ctx, org_config_obj, flow.Context.ClientId),
			flow.Context.SessionId,
			flow.Context.StartTime / 1000,
			flow.Context.State.String(),
			flow.Context.ExecutionDuration / 1000000000,
			flow.Context.TotalUploadedBytes,
			flow.Context.TotalCollectedRows,
		}

		opts := vjson.DefaultEncOpts()
		serialized, err := json.MarshalWithOptions(row_data, opts)
		if err != nil {
			continue
		}

		result.Rows = append(result.Rows, &api_proto.Row{
			Json: string(serialized),
		})

		if uint64(len(result.Rows)) > in.Rows {
			break
		}
	}
	return result, nil
}

func (self *ApiServer) GetHuntTable(
	ctx context.Context,
	in *api_proto.GetTableRequest) (*api_proto.GetTableResponse, error) {

	defer Instrument("GetHuntTable")()

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

	options, err := tables.GetTableOptions(in)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	hunts, total, err := hunt_dispatcher.GetHunts(ctx, org_config_obj, options,
		int64(in.StartRow), int64(in.Rows))
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	result := &api_proto.GetTableResponse{
		TotalRows: total,
		Columns: []string{
			"State", "Tags", "HuntId",
			"Description", "Created",
			"Started", "Expires", "Scheduled", "Creator",
		}}

	for _, hunt := range hunts {
		var total_clients_scheduled uint64
		if hunt.Stats != nil {
			total_clients_scheduled = hunt.Stats.TotalClientsScheduled
		}

		row_data := []interface{}{
			fmt.Sprintf("%v", hunt.State),
			hunt.Tags,
			hunt.HuntId,
			hunt.HuntDescription,
			hunt.CreateTime,
			hunt.StartTime,
			hunt.Expires,
			total_clients_scheduled,
			hunt.Creator,
		}
		opts := vjson.DefaultEncOpts()
		serialized, err := json.MarshalWithOptions(row_data, opts)
		if err != nil {
			continue
		}
		result.Rows = append(result.Rows, &api_proto.Row{
			Json: string(serialized),
		})

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

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	var orgs_we_scheduled []string
	var errors_msg []string

	for _, org_id := range orgs {
		org_config_obj, err := org_manager.GetOrgConfig(org_id)
		if err != nil {
			errors_msg = append(errors_msg, fmt.Sprintf("In Org %v: GetOrgConfig %v", org_id, err))
			continue
		}

		// Make sure the user is allowed to collect in that org
		perm, err := services.CheckAccess(
			org_config_obj, in.Creator, permissions)
		if !perm {
			if err != nil {
				errors_msg = append(errors_msg, fmt.Sprintf(
					"%v: CreateHunt: User is not allowed to launch hunts in "+
						"org %v.", err, org_id))
			} else {
				errors_msg = append(errors_msg, fmt.Sprintf(
					"CreateHunt: User is not allowed to launch hunts in "+
						"org %v.", org_id))
			}
			continue
		}

		hunt_dispatcher, err := services.GetHuntDispatcher(org_config_obj)
		if err != nil {
			errors_msg = append(errors_msg, fmt.Sprintf(
				"%v: CreateHunt: GetOrgConfig %v", org_id, err))
			continue
		}

		// In the root org mark the org ids that we are launching.
		org_hunt_request := proto.Clone(in).(*api_proto.Hunt)
		if !utils.IsRootOrg(org_id) {
			org_hunt_request.OrgIds = nil
		}

		new_hunt, err := hunt_dispatcher.CreateHunt(
			ctx, org_config_obj, acl_manager, org_hunt_request)
		if err != nil {
			errors_msg = append(errors_msg, fmt.Sprintf(
				"%v: CreateHunt: GetOrgConfig %v", org_id, err))
			continue
		}

		orgs_we_scheduled = append(orgs_we_scheduled, org_id)
		// Reuse the hunt id for all the hunts we launch on all the
		// orgs - this makes it easier to combine results from all
		// orgs.
		in.HuntId = new_hunt.HuntId
	}

	if len(errors_msg) != 0 {
		return nil, Status(self.verbose,
			errors.New(strings.Join(errors_msg, "\n")))
	}

	result := &api_proto.StartFlowResponse{}
	result.FlowId = in.HuntId

	// Audit message for GUI access
	err = services.LogAudit(ctx,
		org_config_obj, principal, "CreateHunt",
		ordereddict.NewDict().
			Set("hunt_id", result.FlowId).
			Set("details", in).
			Set("orgs", orgs_we_scheduled))
	if err != nil {
		logger := logging.GetLogger(org_config_obj, &logging.FrontendComponent)
		logger.Error("<red>CreateHunt</> %v %v", principal, result.FlowId)
	}

	return result, nil
}

func (self *ApiServer) ModifyHunt(
	ctx context.Context,
	in *api_proto.HuntMutation) (*emptypb.Empty, error) {

	defer Instrument("ModifyHunt")()

	// Log this event as an Audit event.
	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.COLLECT_CLIENT
	if in.State == api_proto.Hunt_RUNNING {
		permissions = acls.START_HUNT
	}

	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to modify hunts.")
	}

	err = services.LogAudit(ctx,
		org_config_obj, principal, "ModifyHunt",
		ordereddict.NewDict().
			Set("hunt_id", in.HuntId).
			Set("details", in))
	if err != nil {
		logger := logging.GetLogger(org_config_obj, &logging.FrontendComponent)
		logger.Error("<red>ModifyHunt</> %v %v", principal, in.HuntId)
	}

	hunt_dispatcher, err := services.GetHuntDispatcher(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	// Only allow some fields to be set by the GUI
	mutation := &api_proto.HuntMutation{
		HuntId:      in.HuntId,
		State:       in.State,
		Description: in.Description,
		Stats:       in.Stats,
		Expires:     in.Expires,
		Tags:        in.Tags,
		User:        principal,
	}

	err = hunt_dispatcher.MutateHunt(ctx, org_config_obj, mutation)
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

	result, pres := hunt_dispatcher.GetHunt(ctx, in.HuntId)
	if !pres {
		return nil, Status(self.verbose,
			fmt.Errorf("%w: %v", services.HuntNotFoundError, in.HuntId))
	}

	if !in.IncludeRequest && result.StartRequest != nil {
		result.StartRequest.CompiledCollectorArgs = nil
	}

	return result, nil
}

func (self *ApiServer) GetHuntTags(
	ctx context.Context,
	in *emptypb.Empty) (*api_proto.HuntTags, error) {
	defer Instrument("GetHuntTags")()

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

	return &api_proto.HuntTags{
		Tags: hunt_dispatcher.GetTags(ctx),
	}, nil
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
