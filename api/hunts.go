package api

import (
	"fmt"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	file_store "www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/flows"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/search"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

func (self *ApiServer) GetHuntFlows(
	ctx context.Context,
	in *api_proto.GetTableRequest) (*api_proto.GetTableResponse, error) {

	user_name := GetGRPCUserInfo(self.config, ctx, self.ca_pool).Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view hunt results.")
	}

	hunt_path_manager := paths.NewHuntPathManager(in.HuntId).Clients()
	file_store_factory := file_store.GetFileStore(self.config)
	rs_reader, err := result_sets.NewResultSetReader(
		file_store_factory, hunt_path_manager)
	if err != nil {
		return nil, err
	}
	defer rs_reader.Close()

	// Seek to the row we need.
	err = rs_reader.SeekToRow(int64(in.StartRow))
	if err != nil {
		return nil, err
	}

	result := &api_proto.GetTableResponse{
		TotalRows: rs_reader.TotalRows(),
		Columns: []string{
			"ClientId", "Hostname", "FlowId", "StartedTime", "State", "Duration",
			"TotalBytes", "TotalRows",
		}}

	for row := range rs_reader.Rows(ctx) {
		client_id := utils.GetString(row, "ClientId")
		flow_id := utils.GetString(row, "FlowId")
		flow, err := flows.LoadCollectionContext(self.config, client_id, flow_id)
		if err != nil {
			continue
		}

		row_data := []string{
			client_id,
			services.GetHostname(client_id),
			flow_id,
			csv.AnyToString(flow.StartTime / 1000),
			flow.State.String(),
			csv.AnyToString(flow.ExecutionDuration / 1000000000),
			csv.AnyToString(flow.TotalUploadedBytes),
			csv.AnyToString(flow.TotalCollectedRows)}

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

	// Log this event as an Audit event.
	in.Creator = GetGRPCUserInfo(self.config, ctx, self.ca_pool).Name
	in.HuntId = flows.GetNewHuntId()

	acl_manager := vql_subsystem.NewServerACLManager(self.config, in.Creator)

	permissions := acls.COLLECT_CLIENT
	perm, err := acls.CheckAccess(self.config, in.Creator, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to launch hunts.")
	}

	logging.GetLogger(self.config, &logging.Audit).
		WithFields(logrus.Fields{
			"user":    in.Creator,
			"hunt_id": in.HuntId,
			"details": fmt.Sprintf("%v", in),
		}).Info("CreateHunt")

	result := &api_proto.StartFlowResponse{}
	hunt_id, err := flows.CreateHunt(
		ctx, self.config, acl_manager, in)
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
	in.Creator = GetGRPCUserInfo(self.config, ctx, self.ca_pool).Name

	permissions := acls.COLLECT_CLIENT
	perm, err := acls.CheckAccess(self.config, in.Creator, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to modify hunts.")
	}

	logging.GetLogger(self.config, &logging.Audit).
		WithFields(logrus.Fields{
			"user":    in.Creator,
			"hunt_id": in.HuntId,
			"details": fmt.Sprintf("%v", in),
		}).Info("ModifyHunt")

	err = flows.ModifyHunt(ctx, self.config, in, in.Creator)
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

	user_name := GetGRPCUserInfo(self.config, ctx, self.ca_pool).Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view hunts.")
	}

	result, err := flows.ListHunts(self.config, in)
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

	user_name := GetGRPCUserInfo(self.config, ctx, self.ca_pool).Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view hunts.")
	}

	result, err := flows.GetHunt(self.config, in)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (self *ApiServer) GetHuntResults(
	ctx context.Context,
	in *api_proto.GetHuntResultsRequest) (*api_proto.GetTableResponse, error) {

	defer Instrument("GetHuntResults")()

	user_name := GetGRPCUserInfo(self.config, ctx, self.ca_pool).Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
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
	result, err := RunVQL(ctx, self.config, user_name, env,
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

	user_name := GetGRPCUserInfo(self.config, ctx, self.ca_pool).Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view hunt results.")
	}

	client_info_manager, err := services.GetClientInfoManager()
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
				for entity := range search.SearchIndexWithPrefix(
					ctx, self.config, "label:"+label) {
					is_client_recent(entity.Entity, seen)
				}
			}

			// Remove any excluded labels.
			if in.Condition.ExcludedLabels != nil {
				for _, label := range in.Condition.ExcludedLabels.Label {
					for entity := range search.SearchIndexWithPrefix(
						ctx, self.config, "label:"+label) {
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

			client_info_manager, err := services.GetClientInfoManager()
			if err != nil {
				return nil, err
			}

			for hit := range search.SearchIndexWithPrefix(ctx,
				self.config, "all") {
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
					for entity := range search.SearchIndexWithPrefix(
						ctx, self.config, "label:"+label) {
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
		for hit := range search.SearchIndexWithPrefix(ctx, self.config, "all") {
			is_client_recent(hit.Entity, seen)
		}

		// Remove any excluded labels.
		if in.Condition.ExcludedLabels != nil {
			for _, label := range in.Condition.ExcludedLabels.Label {
				for entity := range search.SearchIndexWithPrefix(
					ctx, self.config, "label:"+label) {
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
	for hit := range search.SearchIndexWithPrefix(ctx, self.config, "all") {
		is_client_recent(hit.Entity, seen)
	}

	return &api_proto.HuntStats{
		TotalClientsScheduled: uint64(len(seen)),
	}, nil
}
