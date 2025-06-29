package clients

import (
	"context"
	"sync"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type DeleteClientArgs struct {
	ClientId   string `vfilter:"required,field=client_id"`
	ReallyDoIt bool   `vfilter:"optional,field=really_do_it"`
}

type DeleteClientPlugin struct{}

func (self DeleteClientPlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {

	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "client_delete", args)()

		arg := &DeleteClientArgs{}

		err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
		if err != nil {
			scope.Log("client_delete: %s", err)
			return
		}

		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("client_delete: %s", err)
			return
		}

		if !constants.ClientIdRegex.MatchString(arg.ClientId) {
			scope.Log("ERROR:client_delete: Client Id '%s' should be of the form C.XXXX", arg.ClientId)
			return
		}

		err = services.RequireFrontend()
		if err != nil {
			scope.Log("client_delete: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("client_delete: Command can only run on the server")
			return
		}

		client_info_manager, err := services.GetClientInfoManager(config_obj)
		if err != nil {
			scope.Log("client_delete: %v", err)
			return
		}

		principal := vql_subsystem.GetPrincipal(scope)

		progress := make(chan services.DeleteFlowResponse)
		wg := sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer wg.Done()

			for item := range progress {
				var vfs_path string
				if item.Data != nil {
					vfs_path, _ = item.Data.GetString("vfs_path")
				}
				output_chan <- ordereddict.NewDict().
					Set("client_id", arg.ClientId).
					Set("type", item.Type).
					Set("vfs_path", vfs_path).
					Set("error", item.Error)
			}
		}()

		_ = client_info_manager.DeleteClient(ctx, arg.ClientId, principal,
			progress, arg.ReallyDoIt)

		close(progress)
		wg.Wait()
	}()

	return output_chan
}

func (self DeleteClientPlugin) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "client_delete",
		Doc:      "Delete all information related to a client. ",
		ArgType:  type_map.AddType(scope, &DeleteClientArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.DELETE_RESULTS).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&DeleteClientPlugin{})
}
