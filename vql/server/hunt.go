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
package server

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"

	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/vfilter"
)

type ScheduleHuntFunctionArg struct {
	Description string      `vfilter:"required,field=description,doc=Description of the hunt"`
	Artifacts   []string    `vfilter:"required,field=artifacts,doc=A list of artifacts to collect"`
	Env         vfilter.Any `vfilter:"optional,field=env,doc=Parameters to apply to the artifacts"`
}

type ScheduleHuntFunction struct{}

func (self *ScheduleHuntFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.COLLECT_CLIENT)
	if err != nil {
		scope.Log("flows: %s", err)
		return vfilter.Null{}
	}

	arg := &ScheduleHuntFunctionArg{}
	err = vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("hunt: %s", err.Error())
		return vfilter.Null{}
	}

	any_config_obj, _ := scope.Resolve("server_config")
	config_obj, ok := any_config_obj.(*config_proto.Config)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	request := &flows_proto.ArtifactCollectorArgs{
		Artifacts: arg.Artifacts,
	}

	for _, k := range scope.GetMembers(arg.Env) {
		value, pres := scope.Associative(arg.Env, k)
		if pres {
			value_str, ok := value.(string)
			if !ok {
				scope.Log("hunt: Env must be a dict of strings")
				return vfilter.Null{}
			}

			request.Parameters.Env = append(request.Parameters.Env,
				&actions_proto.VQLEnv{
					Key: k, Value: value_str,
				})
		}
	}

	hunt_request := &api_proto.Hunt{
		HuntDescription: arg.Description,
		StartRequest:    request,
		State:           api_proto.Hunt_RUNNING,
	}

	channel := grpc_client.GetChannel(config_obj)
	defer channel.Close()

	client := api_proto.NewAPIClient(channel)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	response, err := client.CreateHunt(ctx, hunt_request)
	if err != nil {
		scope.Log("hunt: %s", err.Error())
		return vfilter.Null{}
	}

	return response
}

func (self ScheduleHuntFunction) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "hunt",
		Doc:     "Launch an artifact collection against a client.",
		ArgType: type_map.AddType(scope, &ScheduleHuntFunctionArg{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&ScheduleHuntFunction{})
}
