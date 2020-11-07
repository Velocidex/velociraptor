package api

import (
	context "golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	file_store "www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/file_store/result_sets"
	"www.velocidex.com/golang/velociraptor/flows"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/utils"
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
			"ClientId", "FlowId", "StartedTime", "State", "Duration",
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
			client_id, flow_id,
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
