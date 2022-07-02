package flows

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type DeleteFlowPluginArgs struct {
	FlowId     string `vfilter:"required,field=flow_id"`
	ClientId   string `vfilter:"required,field=client_id"`
	ReallyDoIt bool   `vfilter:"optional,field=really_do_it"`
}

type DeleteFlowPlugin struct{}

func (self DeleteFlowPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
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

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		launcher, err := services.GetLauncher(config_obj)
		if err != nil {
			scope.Log("delete_flow: %v", err)
			return
		}

		responses, err := launcher.DeleteFlow(ctx, config_obj,
			arg.ClientId, arg.FlowId, arg.ReallyDoIt)
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
		Name:    "delete_flow",
		Doc:     "Delete all the files that make up a flow.",
		ArgType: type_map.AddType(scope, &FlowsPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&DeleteFlowPlugin{})
}
