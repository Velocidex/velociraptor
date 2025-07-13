package notebooks

import (
	"context"
	"encoding/base64"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type UpdateNotebookCellFunctionArgs struct {
	NotebookId string `vfilter:"required,field=notebook_id,doc=The id of the notebook to update"`
	CellId     string `vfilter:"optional,field=cell_id,doc=The cell of the notebook to update. If this is empty we add a new cell to the notebook"`
	Delete     bool   `vfilter:"optional,field=delete,doc=If set the notebook cell is removed from the notebook."`
	Type       string `vfilter:"optional,field=type,doc=Set the type of the cell if needed (markdown or vql)."`
	Input      string `vfilter:"optional,field=input,doc=The new cell content."`
	Output     string `vfilter:"optional,field=output,doc=If this is set, we do not calculate the cell but set this as the rendered output."`
}

type UpdateNotebookCellFunction struct{}

func (self UpdateNotebookCellFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.COLLECT_SERVER)
	if err != nil {
		scope.Log("notebook_update_cell: %v", err)
		return vfilter.Null{}
	}

	arg := &UpdateNotebookCellFunctionArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("notebook_update_cell: %v", err)
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("notebook_update_cell: %v", err)
		return vfilter.Null{}
	}

	config_obj, pres := vql_subsystem.GetServerConfig(scope)
	if !pres {
		scope.Log("notebook_update_cell: must be running on the server")
		return vfilter.Null{}
	}

	if arg.Delete {
		res, err := self.deleteCell(ctx, config_obj, scope, arg.NotebookId, arg.CellId)
		if err != nil {
			scope.Log("notebook_update_cell: %v", err)
			return vfilter.Null{}
		}
		return res
	}

	notebook_manager, err := services.GetNotebookManager(config_obj)
	if err != nil {
		scope.Log("notebook_update_cell: %v", err)
		return vfilter.Null{}
	}

	request := &api_proto.NotebookCellRequest{
		NotebookId: arg.NotebookId,
		CellId:     arg.CellId,
		Input:      arg.Input,
		Output:     arg.Output,
		Type:       arg.Type,
	}

	principal := vql_subsystem.GetPrincipal(scope)
	var notebook *api_proto.NotebookMetadata
	// Create a new cell
	if arg.CellId == "" {
		notebook, err = notebook_manager.NewNotebookCell(ctx,
			request, principal)
		if err != nil {
			scope.Log("notebook_update_cell: %v", err)
			return vfilter.Null{}
		}

		arg.CellId = notebook.LatestCellId

		// Update an existing cell
	} else {
		notebook, err = notebook_manager.GetNotebook(
			ctx, arg.NotebookId, services.DO_NOT_INCLUDE_UPLOADS)
		if err != nil {
			scope.Log("notebook_update_cell: %v", err)
			return vfilter.Null{}
		}

		_, err = notebook_manager.UpdateNotebookCell(
			ctx, notebook, principal, request)
		if err != nil {
			scope.Log("notebook_update_cell: %v", err)
			return vfilter.Null{}
		}

		// Get the updated notebook
		notebook, err = notebook_manager.GetNotebook(
			ctx, arg.NotebookId, services.DO_NOT_INCLUDE_UPLOADS)
		if err != nil {
			scope.Log("notebook_update_cell: %v", err)
			return vfilter.Null{}
		}

		arg.CellId = notebook.LatestCellId
	}

	// Refresh the cell with fresh data
	err = fillNotebookCells(ctx, config_obj, notebook)
	if err != nil {
		scope.Log("notebook_update_cell: %v", err)
		return vfilter.Null{}
	}

	err = services.LogAudit(ctx,
		config_obj, principal, "UpdateNotebookCell",
		ordereddict.NewDict().
			Set("notebook_id", notebook.NotebookId).
			Set("cell_id", arg.CellId).
			Set("details", vfilter.RowToDict(ctx, scope, arg)))
	if err != nil {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Error("<red>UpdateNotebookCell</> %v %v", principal, notebook.NotebookId)
	}

	return notebook
}

func (self UpdateNotebookCellFunction) deleteCell(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	notebook_id, cell_id string,
) (*api_proto.NotebookMetadata, error) {

	notebook_manager, err := services.GetNotebookManager(config_obj)
	if err != nil {
		return nil, err
	}

	notebook, err := notebook_manager.GetNotebook(
		ctx, notebook_id, services.DO_NOT_INCLUDE_UPLOADS)
	if err != nil {
		return nil, err
	}

	var new_cells []*api_proto.NotebookCell
	for _, c := range notebook.CellMetadata {
		if c.CellId != cell_id {
			new_cells = append(new_cells, c)
		}
	}

	notebook.CellMetadata = new_cells
	return notebook, notebook_manager.UpdateNotebook(ctx, notebook)

}

func (self UpdateNotebookCellFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "notebook_update_cell",
		Doc:     "Update a notebook cell.",
		ArgType: type_map.AddType(scope, &UpdateNotebookCellFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(
			acls.COLLECT_SERVER).Build(),
	}
}

type UpdateNotebookFunctionArgs struct {
	NotebookId    string   `vfilter:"required,field=notebook_id,doc=The id of the notebook to update"`
	Description   string   `vfilter:"optional,field=description,doc=The description of the notebook"`
	Collaborators []string `vfilter:"optional,field=collaborators,doc=A list of users to share the notebook with."`
	Public        bool     `vfilter:"optional,field=public,doc=If set the notebook will be public."`

	Attachment         string `vfilter:"optional,field=attachment,doc=Raw data of an attachment to be added to the notebook"`
	AttachmentFilename string `vfilter:"optional,field=attachment_filename,doc=The name of the attachment"`
}

type UpdateNotebookFunction struct{}

func (self UpdateNotebookFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.COLLECT_SERVER)
	if err != nil {
		scope.Log("notebook_update: %v", err)
		return vfilter.Null{}
	}

	arg := &UpdateNotebookFunctionArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("notebook_update: %v", err)
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("notebook_update: %v", err)
		return vfilter.Null{}
	}

	config_obj, pres := vql_subsystem.GetServerConfig(scope)
	if !pres {
		scope.Log("notebook_update: must be running on the server")
		return vfilter.Null{}
	}

	notebook_manager, err := services.GetNotebookManager(config_obj)
	if err != nil {
		scope.Log("notebook_update: %v", err)
		return vfilter.Null{}
	}

	notebook, err := notebook_manager.GetNotebook(
		ctx, arg.NotebookId, services.DO_NOT_INCLUDE_UPLOADS)
	if err != nil {
		scope.Log("notebook_update: %v", err)
		return vfilter.Null{}
	}

	_, pres = args.Get("description")
	if pres {
		notebook.Description = arg.Description
	}

	_, pres = args.Get("Collaborators")
	if pres {
		notebook.Collaborators = arg.Collaborators
	}

	_, pres = args.Get("Public")
	if pres {
		notebook.Public = arg.Public
	}

	if arg.Attachment != "" {
		data := base64.StdEncoding.EncodeToString(
			[]byte(arg.Attachment))
		if arg.AttachmentFilename == "" {
			arg.AttachmentFilename = "Att" + utils.NextId()
		}
		_, err = notebook_manager.UploadNotebookAttachment(ctx,
			&api_proto.NotebookFileUploadRequest{
				NotebookId: arg.NotebookId,
				Filename:   arg.AttachmentFilename,
				Data:       data,
			})
		if err != nil {
			scope.Log("notebook_update: %v", err)
			return vfilter.Null{}
		}
	}

	err = notebook_manager.UpdateNotebook(ctx, notebook)
	if err != nil {
		scope.Log("notebook_update: %v", err)
		return vfilter.Null{}
	}

	principal := vql_subsystem.GetPrincipal(scope)
	err = services.LogAudit(ctx,
		config_obj, principal, "UpdateNotebookCell",
		ordereddict.NewDict().
			Set("notebook_id", notebook.NotebookId).
			Set("details", vfilter.RowToDict(ctx, scope, arg)))
	if err != nil {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Error("<red>UpdateNotebookCell</> %v %v", principal, notebook.NotebookId)
	}

	return notebook
}

func (self UpdateNotebookFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "notebook_update",
		Doc:     "Update a notebook metadata.",
		ArgType: type_map.AddType(scope, &UpdateNotebookFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(
			acls.COLLECT_SERVER).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&UpdateNotebookFunction{})
	vql_subsystem.RegisterFunction(&UpdateNotebookCellFunction{})
}
