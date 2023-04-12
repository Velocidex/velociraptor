package hunts

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type UpdateHuntFunctionArg struct {
	HuntId      string `vfilter:"required,field=hunt_id,doc=The hunt to update"`
	Stop        bool   `vfilter:"optional,field=stop,doc=Stop the hunt"`
	Start       bool   `vfilter:"optional,field=start,doc=Start the hunt"`
	Description string `vfilter:"optional,field=description,doc=Update hunt description"`
}

type UpdateHuntFunction struct{}

func (self *UpdateHuntFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.START_HUNT)
	if err != nil {
		scope.Log("hunt_update: %v", err)
		return vfilter.Null{}
	}

	arg := &UpdateHuntFunctionArg{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("hunt_update: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("hunt_update: GetServerConfig not found")
		return vfilter.Null{}
	}

	hunt_dispatcher, err := services.GetHuntDispatcher(config_obj)
	if err != nil {
		scope.Log("hunt_update: %v", err)
		return vfilter.Null{}
	}

	if arg.Start || arg.Stop {
		state := api_proto.Hunt_STOPPED
		if arg.Start {
			state = api_proto.Hunt_RUNNING
		}

		err = hunt_dispatcher.MutateHunt(
			ctx, config_obj, &api_proto.HuntMutation{
				HuntId: arg.HuntId,
				State:  state,
			})
		if err != nil {
			scope.Log("hunt_update: %v", err)
			return vfilter.Null{}
		}
	}

	if arg.Description != "" {
		err = hunt_dispatcher.MutateHunt(
			ctx, config_obj, &api_proto.HuntMutation{
				HuntId:      arg.HuntId,
				Description: arg.Description,
			})
		if err != nil {
			scope.Log("hunt_update: %v", err)
			return vfilter.Null{}
		}
	}

	return arg.HuntId
}

func (self UpdateHuntFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "hunt_update",
		Doc:      "Update a hunt.",
		ArgType:  type_map.AddType(scope, &UpdateHuntFunctionArg{}),
		Metadata: vql.VQLMetadata().Permissions(acls.START_HUNT).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&UpdateHuntFunction{})
}
