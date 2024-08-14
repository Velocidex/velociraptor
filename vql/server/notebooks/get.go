package notebooks

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type GetNotebookFunctionArg struct {
	NotebookId string `vfilter:"required,field=notebook_id,doc=The id of the notebook to fetch"`
	Verbose    bool   `vfilter:"optional,field=verbose,doc=Include more information"`
}

type GetNotebookFunction struct{}

func (self GetNotebookFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
	if err != nil {
		scope.Log("notebook_get: %v", err)
		return vfilter.Null{}
	}

	arg := &GetNotebookFunctionArg{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("notebook_get: %v", err)
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("notebook_get: %v", err)
		return vfilter.Null{}
	}

	config_obj, pres := vql_subsystem.GetServerConfig(scope)
	if !pres {
		scope.Log("notebook_get: must be running on the server")
		return vfilter.Null{}
	}

	notebook_manager, err := services.GetNotebookManager(config_obj)
	if err != nil {
		scope.Log("notebook_get: %v", err)
		return vfilter.Null{}
	}

	uploads_flag := services.DO_NOT_INCLUDE_UPLOADS
	if arg.Verbose {
		uploads_flag = services.INCLUDE_UPLOADS
	}

	notebook, err := notebook_manager.GetNotebook(
		ctx, arg.NotebookId, uploads_flag)
	if err != nil {
		scope.Log("notebook_get: %v", err)
		return vfilter.Null{}
	}

	err = fillNotebookCells(ctx, config_obj, notebook)
	if err != nil {
		scope.Log("notebook_get: %v", err)
		return vfilter.Null{}
	}

	return notebook
}

// Populate the cells in a notebook summary.
func fillNotebookCells(
	ctx context.Context, config_obj *config_proto.Config,
	summary_notebook *api_proto.NotebookMetadata) error {

	notebook_manager, err := services.GetNotebookManager(config_obj)
	if err != nil {
		return err
	}

	// Now fill in the content of all the cells.
	for idx, metadata := range summary_notebook.CellMetadata {
		if metadata.CellId == "" {
			continue
		}
		cell, err := notebook_manager.GetNotebookCell(ctx,
			summary_notebook.NotebookId, metadata.CellId,
			metadata.CurrentVersion)
		if err != nil {
			continue
		}
		summary_notebook.CellMetadata[idx] = cell
	}

	return nil
}

func (self GetNotebookFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "notebook_get",
		Doc:     "Get a notebook.",
		ArgType: type_map.AddType(scope, &GetNotebookFunctionArg{}),
		Metadata: vql.VQLMetadata().Permissions(
			acls.READ_RESULTS).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&GetNotebookFunction{})
}
