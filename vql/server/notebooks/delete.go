package notebooks

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type DeleteNotebookArgs struct {
	NotebookId string `vfilter:"required,field=notebook_id"`
	ReallyDoIt bool   `vfilter:"optional,field=really_do_it"`
}

type DeleteNotebookPlugin struct{}

func (self *DeleteNotebookPlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {

	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "notebook_delete", args)()

		arg := &DeleteNotebookArgs{}

		err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
		if err != nil {
			scope.Log("notebook_delete: %s", err)
			return
		}

		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("notebook_delete: %s", err.Error())
			return
		}

		err = services.RequireFrontend()
		if err != nil {
			scope.Log("notebook_delete: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("notebook_delete: Command can only run on the server")
			return
		}

		notebook_manager, err := services.GetNotebookManager(config_obj)
		if err != nil {
			scope.Log("notebook_delete: %v", err)
			return
		}

		err = notebook_manager.DeleteNotebook(ctx, arg.NotebookId,
			output_chan, arg.ReallyDoIt)
		if err != nil {
			scope.Log("notebook_delete: %v", err)
			return
		}
	}()

	return output_chan
}

func (self DeleteNotebookPlugin) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "notebook_delete",
		Doc:      "Delete a notebook with all its cells. ",
		ArgType:  type_map.AddType(scope, &DeleteNotebookArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.SERVER_ADMIN).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&DeleteNotebookPlugin{})
}
