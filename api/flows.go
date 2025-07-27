package api

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"google.golang.org/protobuf/types/known/emptypb"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/api/tables"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/json"
	vjson "www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
)

func (self *ApiServer) CancelFlow(
	ctx context.Context,
	in *api_proto.ApiFlowRequest) (*api_proto.StartFlowResponse, error) {

	defer Instrument("CancelFlow")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.COLLECT_CLIENT
	if in.ClientId == "server" {
		permissions = acls.COLLECT_SERVER
	}

	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to cancel flows.")
	}

	launcher, err := services.GetLauncher(org_config_obj)
	if err != nil {
		return nil, err
	}
	result, err := launcher.CancelFlow(
		ctx, org_config_obj, in.ClientId, in.FlowId, principal)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	// Log this event as and Audit event.
	err = services.LogAudit(ctx,
		org_config_obj, principal, "CancelFlow",
		ordereddict.NewDict().
			Set("client", in.ClientId).
			Set("flow_id", in.FlowId).
			Set("details", in))

	return result, err
}

func (self *ApiServer) ResumeFlow(
	ctx context.Context,
	in *api_proto.ApiFlowRequest) (*emptypb.Empty, error) {

	defer Instrument("ResumeFlow")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.COLLECT_CLIENT
	if in.ClientId == "server" {
		permissions = acls.COLLECT_SERVER
	}

	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to resume flows.")
	}

	launcher, err := services.GetLauncher(org_config_obj)
	if err != nil {
		return nil, err
	}
	_, err = launcher.ResumeFlow(
		ctx, org_config_obj, in.ClientId, in.FlowId)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	// Log this event as and Audit event.
	err = services.LogAudit(ctx,
		org_config_obj, principal, "ResumeFlow",
		ordereddict.NewDict().
			Set("client", in.ClientId).
			Set("flow_id", in.FlowId).
			Set("details", in))

	return &emptypb.Empty{}, err
}

func (self *ApiServer) GetClientFlows(
	ctx context.Context,
	in *api_proto.GetTableRequest) (*api_proto.GetTableResponse, error) {

	defer Instrument("GetClientFlows")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	user_name := user_record.Name
	permissions := acls.READ_RESULTS
	perm, err := services.CheckAccess(org_config_obj, user_name, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to view flows.")
	}

	launcher, err := services.GetLauncher(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	// If no sort column is specified, sort by flow id so later flows
	// are on top. Flow Ids have times encoded in them so they sort
	// chronologically.
	if in.SortColumn == "" {
		in.SortColumn = "FlowId"
		in.SortDirection = true
	}

	options, err := tables.GetTableOptions(in)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	flows, err := launcher.GetFlows(ctx, org_config_obj, in.ClientId, options,
		int64(in.StartRow), int64(in.Rows))
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	result := &api_proto.GetTableResponse{
		TotalRows: int64(flows.Total),
		Columns: []string{
			"State", "FlowId", "Artifacts", "Created", "Last Active", "Creator",
			"Mb", "Rows", "_Flow", "_Urgent", "_ArtifactsWithResults",
		},
		ColumnTypes: []*artifacts_proto.ColumnType{{
			Name: "Created",
			Type: "timestamp",
		}, {
			Name: "Last Active",
			Type: "timestamp",
		}, {
			Name: "Mb",
			Type: "mb",
		}, {
			Name: "Rows",
			Type: "number",
		}},
	}

	// Convert the items into a table format
	for _, flow := range flows.Items {
		if flow.Request == nil {
			continue
		}
		row_data := []interface{}{
			flow.State.String(),
			flow.SessionId,
			flow.Request.Artifacts,
			flow.CreateTime,
			flow.ActiveTime,
			flow.Request.Creator,
			flow.TotalUploadedBytes,
			flow.TotalCollectedRows,
			json.ConvertProtoToOrderedDict(flow),
			flow.Request.Urgent,
			flow.ArtifactsWithResults,
		}
		opts := vjson.DefaultEncOpts()
		serialized, err := json.MarshalWithOptions(row_data, opts)
		if err != nil {
			continue
		}
		result.Rows = append(result.Rows, &api_proto.Row{
			Json: string(serialized),
		})
	}

	return result, nil
}
