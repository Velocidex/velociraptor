package hunts

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type DeleteHuntArgs struct {
	HuntId     string `vfilter:"required,field=hunt_id"`
	ReallyDoIt bool   `vfilter:"optional,field=really_do_it"`
}

type DeleteHuntPlugin struct{}

func (self DeleteHuntPlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {

	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &DeleteHuntArgs{}

		err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
		if err != nil {
			scope.Log("hunt_delete: %s", err)
			return
		}

		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("hunt_delete: %s", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		launcher, err := services.GetLauncher()
		if err != nil {
			scope.Log("hunt_delete: %s", err)
			return
		}

		hunt_dispatcher, err := services.GetHuntDispatcher(config_obj)
		if err != nil {
			scope.Log("hunt_delete: %s", err)
			return
		}
		for flow_details := range hunt_dispatcher.GetFlows(
			ctx, config_obj, scope, arg.HuntId, 0) {

			results, err := launcher.DeleteFlow(ctx, config_obj,
				flow_details.Context.ClientId,
				flow_details.Context.SessionId, arg.ReallyDoIt)
			if err != nil {
				scope.Log("hunt_delete: %v", err)
				return
			}

			for _, res := range results {
				select {
				case <-ctx.Done():
					return
				case output_chan <- res:
				}
			}
		}

		// Now remove the hunt from the hunt manager
		if arg.ReallyDoIt {
			mutation := api_proto.HuntMutation{
				HuntId: arg.HuntId,
				State:  api_proto.Hunt_ARCHIVED,
			}
			journal, err := services.GetJournal(config_obj)
			if err != nil {
				scope.Log("hunt_delete: %s", err)
				return
			}

			journal.PushRowsToArtifactAsync(config_obj,
				ordereddict.NewDict().
					Set("hunt_id", arg.HuntId).
					Set("mutation", mutation),
				"Server.Internal.HuntModification")
		}
	}()

	return output_chan
}

func (self DeleteHuntPlugin) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "hunt_delete",
		Doc:     "Delete a hunt. ",
		ArgType: type_map.AddType(scope, &DeleteHuntArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&DeleteHuntPlugin{})
}
