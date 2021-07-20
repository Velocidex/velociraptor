package api

import (
	"fmt"

	"github.com/Velocidex/ordereddict"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	file_store "www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/flows"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

func (self *ApiServer) GetHuntFlows(
	ctx context.Context,
	in *api_proto.GetTableRequest) (*api_proto.GetTableResponse, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
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
	in.Creator = GetGRPCUserInfo(self.config, ctx).Name
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
	in *api_proto.Hunt) (*empty.Empty, error) {

	defer Instrument("ModifyHunt")()

	// Log this event as an Audit event.
	in.Creator = GetGRPCUserInfo(self.config, ctx).Name

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

	result := &empty.Empty{}
	return result, nil
}

func (self *ApiServer) ListHunts(
	ctx context.Context,
	in *api_proto.ListHuntsRequest) (*api_proto.ListHuntsResponse, error) {

	defer Instrument("ListHunts")()

	user_name := GetGRPCUserInfo(self.config, ctx).Name
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

	return result, nil
}

func (self *ApiServer) GetHunt(
	ctx context.Context,
	in *api_proto.GetHuntRequest) (*api_proto.Hunt, error) {
	if in.HuntId == "" {
		return &api_proto.Hunt{}, nil
	}

	defer Instrument("GetHunt")()

	user_name := GetGRPCUserInfo(self.config, ctx).Name
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

	user_name := GetGRPCUserInfo(self.config, ctx).Name
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
