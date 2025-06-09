package flows

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

type DeleteFlowPluginArgs struct {
	FlowId     string `vfilter:"required,field=flow_id"`
	ClientId   string `vfilter:"required,field=client_id"`
	ReallyDoIt bool   `vfilter:"optional,field=really_do_it"`
	Sync       bool   `vfilter:"optional,field=sync,doc=If specified we ensure data is available immediately"`
}

type DeleteFlowPlugin struct{}

func (self DeleteFlowPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "delete_flow", args)()

		err := vql_subsystem.CheckAccess(scope, acls.DELETE_RESULTS)
		if err != nil {
			scope.Log("delete_flow: %s", err)
			return
		}

		arg := &DeleteFlowPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("delete_flow: %v", err)
			return
		}

		err = services.RequireFrontend()
		if err != nil {
			scope.Log("delete_flow: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("delete_flow: Command can only run on the server")
			return
		}

		launcher, err := services.GetLauncher(config_obj)
		if err != nil {
			scope.Log("delete_flow: %v", err)
			return
		}

		principal := vql_subsystem.GetPrincipal(scope)
		responses, err := launcher.Storage().DeleteFlow(ctx, config_obj,
			arg.ClientId, arg.FlowId, principal, services.DeleteFlowOptions{
				ReallyDoIt: arg.ReallyDoIt,
				Sync:       arg.Sync,
			})
		if err != nil {
			scope.Log("delete_flow: %v", err)
			return
		}

		for _, resp := range responses {
			select {
			case <-ctx.Done():
				return
			case output_chan <- resp:
			}
		}
	}()

	return output_chan
}

func (self DeleteFlowPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "delete_flow",
		Doc:      "Delete all the files that make up a flow.",
		ArgType:  type_map.AddType(scope, &DeleteFlowPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.DELETE_RESULTS).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&DeleteFlowPlugin{})
}
