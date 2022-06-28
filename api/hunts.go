package api

import (
	"fmt"
	"time"

	"github.com/Velocidex/ordereddict"
	errors "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/hunt_dispatcher"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

func (self *ApiServer) GetHuntFlows(
	ctx context.Context,
	in *api_proto.GetTableRequest) (*api_proto.GetTableResponse, error) {

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_record.Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(org_config_obj, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view hunt results.")
	}

	hunt_dispatcher := services.GetHuntDispatcher()
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
			services.GetHostname(org_config_obj, flow.Context.ClientId),
			flow.Context.SessionId,
			csv.AnyToString(flow.Context.StartTime / 1000),
			flow.Context.State.String(),
			csv.AnyToString(flow.Context.ExecutionDuration / 1000000000),
			csv.AnyToString(flow.Context.TotalUploadedBytes),
			csv.AnyToString(flow.Context.TotalCollectedRows)}

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
		return nil, err
	}

	// Log this event as an Audit event.
	in.Creator = user_record.Name
	in.HuntId = hunt_dispatcher.GetNewHuntId()

	acl_manager := vql_subsystem.NewServerACLManager(org_config_obj, in.Creator)

	permissions := acls.COLLECT_CLIENT
	perm, err := acls.CheckAccess(org_config_obj, in.Creator, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to launch hunts.")
	}

	logging.GetLogger(org_config_obj, &logging.Audit).
		WithFields(logrus.Fields{
			"user":    in.Creator,
			"hunt_id": in.HuntId,
			"details": fmt.Sprintf("%v", in),
		}).Info("CreateHunt")

	result := &api_proto.StartFlowResponse{}
	hunt_dispatcher := services.GetHuntDispatcher()
	hunt_id, err := hunt_dispatcher.CreateHunt(
		ctx, org_config_obj, acl_manager, in)
	if err != nil {
		return nil, err
	}

	result.FlowId = hunt_id

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
		return nil, err
	}
	in.Creator = user_record.Name

	permissions := acls.COLLECT_CLIENT
	perm, err := acls.CheckAccess(org_config_obj, in.Creator, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to modify hunts.")
	}

	logging.GetLogger(org_config_obj, &logging.Audit).
		WithFields(logrus.Fields{
			"user":    in.Creator,
			"hunt_id": in.HuntId,
			"details": fmt.Sprintf("%v", in),
		}).Info("ModifyHunt")

	hunt_dispatcher := services.GetHuntDispatcher()
	err = hunt_dispatcher.ModifyHunt(ctx, org_config_obj, in, in.Creator)
	if err != nil {
		return nil, err
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
		return nil, err
	}

	user_name := user_record.Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(org_config_obj, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view hunts.")
	}

	hunt_dispatcher := services.GetHuntDispatcher()
	result, err := hunt_dispatcher.ListHunts(
		ctx, org_config_obj, in)
	if err != nil {
		return nil, err
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
		return nil, err
	}

	user_name := user_record.Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(org_config_obj, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view hunts.")
	}

	hunt_dispatcher := services.GetHuntDispatcher()
	result, pres := hunt_dispatcher.GetHunt(in.HuntId)
	if !pres {
		return nil, errors.New("Hunt not found")
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
		return nil, err
	}

	user_name := user_record.Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(org_config_obj, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view results.")
	}

	env := ordereddict.NewDict().
		Set("HuntID", in.HuntId).
		Set("ArtifactName", in.Artifact)

	// More than 100 results are not very useful in the GUI -
	// users should just download the json file for post
	// processing or process in the notebook.
	result, err := RunVQL(ctx, org_config_obj, user_name, env,
		"SELECT * FROM hunt_results(hunt_id=HuntID, "+
			"artifact=ArtifactName) LIMIT 100")
	if err != nil {
		return nil, err
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
		return nil, err
	}

	user_name := user_record.Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(org_config_obj, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view hunt results.")
	}

	client_info_manager, err := services.GetClientInfoManager(org_config_obj)
	if err != nil {
		return nil, err
	}

	indexer, err := services.GetIndexer(org_config_obj)
	if err != nil {
		return nil, err
	}

	now := uint64(time.Now().UnixNano() / 1000)

	is_client_recent := func(client_id string, seen map[string]bool) {
		// We dont care about last active status
		if in.LastActive == 0 {
			seen[client_id] = true
			return
		}

		stats, err := client_info_manager.GetStats(client_id)
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
				return nil, err
			}

			for hit := range indexer.SearchIndexWithPrefix(ctx,
				org_config_obj, "all") {
				client_id := hit.Entity
				client_info, err := client_info_manager.Get(client_id)
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
