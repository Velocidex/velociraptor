package notebooks

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type CreateNotebookFunctionArg struct {
	Name          string            `vfilter:"optional,field=name,doc=The name of the notebook"`
	Description   string            `vfilter:"optional,field=description,doc=The description of the notebook"`
	Collaborators []string          `vfilter:"optional,field=collaborators,doc=A list of users to share the notebook with."`
	Public        bool              `vfilter:"optional,field=public,doc=If set the notebook will be public."`
	Artifacts     []string          `vfilter:"optional,field=artifacts,doc=A list of NOTEBOOK artifacts to create the notebook with (Notebooks.Default)"`
	Env           *ordereddict.Dict `vfilter:"optional,field=env,doc=An environment to initialize the notebook with"`
}

type CreateNotebookFunction struct{}

func (self *CreateNotebookFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.COLLECT_SERVER)
	if err != nil {
		scope.Log("notebook_create: %v", err)
		return vfilter.Null{}
	}

	arg := &CreateNotebookFunctionArg{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("notebook_create: %v", err)
		return vfilter.Null{}
	}

	principal := vql_subsystem.GetPrincipal(scope)
	new_notebook := &api_proto.NotebookMetadata{
		Name:          arg.Name,
		Description:   arg.Description,
		Creator:       principal,
		Collaborators: arg.Collaborators,
		Artifacts:     arg.Artifacts,
		Public:        arg.Public,
	}

	if arg.Env != nil {
		for _, k := range arg.Env.Keys() {
			v := vql_subsystem.GetStringFromRow(scope, arg.Env, k)
			new_notebook.Env = append(new_notebook.Env, &api_proto.Env{
				Key:   k,
				Value: v,
			})
		}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("notebook_create: %v", err)
		return vfilter.Null{}
	}

	config_obj, pres := vql_subsystem.GetServerConfig(scope)
	if !pres {
		scope.Log("notebook_create: must be running on the server")
		return vfilter.Null{}
	}

	notebook_manager, err := services.GetNotebookManager(config_obj)
	if err != nil {
		scope.Log("notebook_create: %v", err)
		return vfilter.Null{}
	}

	new_notebook, err = notebook_manager.NewNotebook(ctx,
		principal, new_notebook)
	if err != nil {
		scope.Log("notebook_create: %v", err)
		return vfilter.Null{}
	}

	err = services.LogAudit(ctx,
		config_obj, principal, "CreateNotebook",
		ordereddict.NewDict().
			Set("notebook_id", new_notebook.NotebookId).
			Set("details", vfilter.RowToDict(ctx, scope, arg)))
	if err != nil {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Error("<red>CreateNotebook</> %v %v", principal, new_notebook.NotebookId)
	}

	err = fillNotebookCells(ctx, config_obj, new_notebook)
	if err != nil {
		scope.Log("notebook_create: %v", err)
		return vfilter.Null{}
	}

	return new_notebook
}

func (self CreateNotebookFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "notebook_create",
		Doc:     "Create a new notebook.",
		ArgType: type_map.AddType(scope, &CreateNotebookFunctionArg{}),
		Metadata: vql.VQLMetadata().Permissions(
			acls.COLLECT_SERVER).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&CreateNotebookFunction{})
}
