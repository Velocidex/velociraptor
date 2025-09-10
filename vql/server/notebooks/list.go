package notebooks

import (
	"context"
	"sort"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type ListNotebookArgs struct {
	All bool `vfilter:"optional,field=all,doc=List all notebooks, not just the ones shared with the user"`
}

type ListNotebookPlugin struct{}

func (self *ListNotebookPlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {

	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "notebooks", args)()

		arg := &ListNotebookArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("notebook: %s", err.Error())
			return
		}

		// Viewing all the notebooks requires server admin
		// permissions, otherwise we just need read permission.
		if arg.All {
			err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
			if err != nil {
				scope.Log("notebooks: %v", err)
				return
			}
		}

		err = vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("notebooks: %v", err)
			return
		}

		err = services.RequireFrontend()
		if err != nil {
			scope.Log("notebooks: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("notebooks: Command can only run on the server")
			return
		}

		notebook_manager, err := services.GetNotebookManager(config_obj)
		if err != nil {
			scope.Log("notebooks: %v", err)
			return
		}

		opts := services.NotebookSearchOptions{}
		if !arg.All {
			opts.Username = vql_subsystem.GetPrincipal(scope)
		}

		all_notebooks, err := notebook_manager.GetAllNotebooks(ctx, opts)
		if err != nil {
			scope.Log("notebooks: %v", err)
			return
		}

		sort.Slice(all_notebooks, func(i, j int) bool {
			return all_notebooks[i].NotebookId > all_notebooks[j].NotebookId
		})

		for _, notebook := range all_notebooks {
			select {
			case <-ctx.Done():
				return
			case output_chan <- json.ConvertProtoToOrderedDict(notebook):
			}
		}

	}()

	return output_chan
}

func (self ListNotebookPlugin) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "notebooks",
		Doc:      "List all notebooks",
		ArgType:  type_map.AddType(scope, &ListNotebookArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.SERVER_ADMIN, acls.READ_RESULTS).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&ListNotebookPlugin{})
}
