// +build server_vql

/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package hunts

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/flows"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/tools"
	"www.velocidex.com/golang/vfilter/arg_parser"

	"www.velocidex.com/golang/vfilter"
)

type ScheduleHuntFunctionArg struct {
	Description   string      `vfilter:"required,field=description,doc=Description of the hunt"`
	Artifacts     []string    `vfilter:"required,field=artifacts,doc=A list of artifacts to collect"`
	Expires       uint64      `vfilter:"optional,field=expires,doc=Number of milliseconds since epoch for expiry"`
	Spec          vfilter.Any `vfilter:"optional,field=spec,doc=Parameters to apply to the artifacts"`
	Timeout       uint64      `vfilter:"optional,field=timeout,doc=Set query timeout (default 10 min)"`
	OpsPerSecond  float64     `vfilter:"optional,field=ops_per_sec,doc=Set query ops_per_sec value"`
	MaxRows       uint64      `vfilter:"optional,field=max_rows,doc=Max number of rows to fetch"`
	MaxBytes      uint64      `vfilter:"optional,field=max_bytes,doc=Max number of bytes to upload"`
	Pause         bool        `vfilter:"optional,field=pause,doc=If specified the new hunt will be in the paused state"`
	IncludeLabels []string    `vfilter:"optional,field=include_labels,doc=If specified only include these labels"`
	ExcludeLabels []string    `vfilter:"optional,field=exclude_labels,doc=If specified exclude these labels"`
}

type ScheduleHuntFunction struct{}

func (self *ScheduleHuntFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.COLLECT_CLIENT)
	if err != nil {
		scope.Log("hunt: %s", err)
		return vfilter.Null{}
	}

	arg := &ScheduleHuntFunctionArg{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("hunt: %s", err.Error())
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	manager, err := services.GetRepositoryManager()
	if err != nil {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}
	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	request := &flows_proto.ArtifactCollectorArgs{
		Creator:        vql_subsystem.GetPrincipal(scope),
		Artifacts:      arg.Artifacts,
		OpsPerSecond:   float32(arg.OpsPerSecond),
		Timeout:        arg.Timeout,
		MaxRows:        arg.MaxRows,
		MaxUploadBytes: arg.MaxBytes,
	}

	err = tools.AddSpecProtobuf(config_obj, repository, scope,
		arg.Spec, request)
	if err != nil {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	state := api_proto.Hunt_RUNNING
	if arg.Pause {
		state = api_proto.Hunt_PAUSED
	}

	hunt_request := &api_proto.Hunt{
		HuntDescription: arg.Description,
		Creator:         vql_subsystem.GetPrincipal(scope),
		StartRequest:    request,
		Expires:         arg.Expires,
		State:           state,
	}

	if len(arg.IncludeLabels) > 0 {
		hunt_request.Condition = &api_proto.HuntCondition{
			UnionField: &api_proto.HuntCondition_Labels{
				Labels: &api_proto.HuntLabelCondition{
					Label: arg.IncludeLabels,
				},
			},
		}

		if len(arg.ExcludeLabels) > 0 {
			hunt_request.Condition.ExcludedLabels = &api_proto.HuntLabelCondition{
				Label: arg.ExcludeLabels,
			}
		}
	}

	// Run the hunt in the ACL context of the caller.
	acl_manager := vql_subsystem.NewServerACLManager(
		config_obj, vql_subsystem.GetPrincipal(scope))
	hunt_id, err := flows.CreateHunt(ctx, config_obj, acl_manager, hunt_request)
	if err != nil {
		scope.Log("hunt: %s", err.Error())
		return vfilter.Null{}
	}

	return ordereddict.NewDict().
		Set("HuntId", hunt_id).
		Set("Request", hunt_request)
}

func (self ScheduleHuntFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "hunt",
		Doc:     "Launch an artifact collection against a client.",
		ArgType: type_map.AddType(scope, &ScheduleHuntFunctionArg{}),
	}
}

type AddToHuntFunctionArg struct {
	ClientId string `vfilter:"required,field=client_id"`
	HuntId   string `vfilter:"required,field=hunt_id"`
	FlowId   string `vfilter:"optional,field=flow_id,doc=If a flow id is specified we do not create a new flow, but instead add this flow_id to the hunt."`
}

type AddToHuntFunction struct{}

func (self *AddToHuntFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.COLLECT_CLIENT)
	if err != nil {
		scope.Log("hunt_add: %s", err)
		return vfilter.Null{}
	}

	arg := &AddToHuntFunctionArg{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("hunt_add: %s", err.Error())
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	journal, _ := services.GetJournal()
	if journal == nil {
		return vfilter.Null{}
	}

	// Send this
	if arg.FlowId != "" {
		err = journal.PushRowsToArtifact(config_obj,
			[]*ordereddict.Dict{ordereddict.NewDict().
				Set("HuntId", arg.HuntId).
				Set("mutation", &api_proto.HuntMutation{
					HuntId: arg.HuntId,
					Assignment: &api_proto.FlowAssignment{
						ClientId: arg.ClientId,
						FlowId:   arg.FlowId,
					},
				})},
			"Server.Internal.HuntModification", arg.ClientId, "")
	} else {
		err = journal.PushRowsToArtifact(config_obj,
			[]*ordereddict.Dict{ordereddict.NewDict().
				Set("HuntId", arg.HuntId).
				Set("ClientId", arg.ClientId).
				Set("Override", true)},
			"System.Hunt.Participation", arg.ClientId, "")
	}

	if err != nil {
		scope.Log("hunt_add: %s", err.Error())
		return vfilter.Null{}
	}

	return arg.ClientId
}

func (self AddToHuntFunction) Info(scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "hunt_add",
		Doc:     "Assign a client to a hunt.",
		ArgType: type_map.AddType(scope, &AddToHuntFunctionArg{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&ScheduleHuntFunction{})
	vql_subsystem.RegisterFunction(&AddToHuntFunction{})
}
