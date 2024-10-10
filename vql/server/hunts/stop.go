package hunts

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type UpdateHuntFunctionArg struct {
	HuntId      string    `vfilter:"required,field=hunt_id,doc=The hunt to update"`
	Stop        bool      `vfilter:"optional,field=stop,doc=Stop the hunt"`
	Start       bool      `vfilter:"optional,field=start,doc=Start the hunt"`
	Description string    `vfilter:"optional,field=description,doc=Update hunt description"`
	Expires     time.Time `vfilter:"optional,field=expires,doc=Update hunt expiry"`
	AddLabel    []string  `vfilter:"optional,field=add_labels,doc=Labels to be added to hunt"`
	DelLabel    []string  `vfilter:"optional,field=del_labels,doc=Labels to be removed from hunt"`
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

	err = services.RequireFrontend()
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

	if arg.Description != "" || !arg.Expires.IsZero() {
		expires := uint64(0)
		if !arg.Expires.IsZero() {
			if utils.GetTime().Now().After(arg.Expires) {
				scope.Log("hunt_update: expiry time spectified is in the past.")
				return vfilter.Null{}
			}
			expires = uint64(arg.Expires.UnixNano()) / 1000
		}

		err = hunt_dispatcher.MutateHunt(
			ctx, config_obj, &api_proto.HuntMutation{
				HuntId:      arg.HuntId,
				Description: arg.Description,
				Expires:     expires,
			})
		if err != nil {
			scope.Log("hunt_update: %v", err)
			return vfilter.Null{}
		}
	}

	if len(arg.AddLabel) > 0 || len(arg.DelLabel) > 0 {
		hunt_obj, pres := hunt_dispatcher.GetHunt(ctx, arg.HuntId)
		if !pres {
			scope.Log("hunt_update: %v", err)
			return &vfilter.Null{}
		}

		tags := hunt_obj.Tags
		for _, tag := range arg.AddLabel {
			if !utils.InStringFolding(tags, tag) {
				tags = append(tags, tag)
			}
		}

		for _, tag := range arg.DelLabel {
			tags = utils.FilterSliceFolding(tags, tag)
		}

		if len(tags) == 0 {
			tags = append(tags, "-")
		}

		err = hunt_dispatcher.MutateHunt(
			ctx, config_obj, &api_proto.HuntMutation{
				HuntId: arg.HuntId,
				Tags:   tags,
			})
		if err != nil {
			scope.Log("hunt_update: %v", err)
			return vfilter.Null{}
		}
		return utils.FilterSlice(tags, "-")
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
