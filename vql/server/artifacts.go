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
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/api"
	"www.velocidex.com/golang/velociraptor/artifacts"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type ScheduleCollectionFunctionArg struct {
	ClientId  string      `vfilter:"required,field=client_id,doc=The client id to schedule a collection on"`
	Artifacts []string    `vfilter:"required,field=artifacts,doc=A list of artifacts to collect"`
	Env       vfilter.Any `vfilter:"optional,field=env,doc=Parameters to apply to the artifacts"`
}

type ScheduleCollectionFunction struct{}

func (self *ScheduleCollectionFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &ScheduleCollectionFunctionArg{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("collect_client: %s", err.Error())
		return vfilter.Null{}
	}

	// Scheduling artifacts on the server requires higher
	// permissions.
	permission := acls.COLLECT_CLIENT
	if arg.ClientId == "server" {
		permission = acls.SERVER_ADMIN
	} else if strings.HasPrefix(arg.ClientId, "C.") {
		permission = acls.COLLECT_CLIENT
	} else {
		scope.Log("collect_client: unsupported client id")
		return vfilter.Null{}
	}

	err = vql_subsystem.CheckAccess(scope, permission)
	if err != nil {
		scope.Log("collect_client: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := artifacts.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}
	request := api.MakeCollectorRequest(arg.ClientId, "")
	request.Artifacts = arg.Artifacts
	request.Creator = vql_subsystem.GetPrincipal(scope)

	for _, k := range scope.GetMembers(arg.Env) {
		value, pres := scope.Associative(arg.Env, k)
		if pres {
			value_str, ok := value.(string)
			if !ok {
				scope.Log("collect_client: Env must be a dict of strings")
				return vfilter.Null{}
			}

			request.Parameters.Env = append(
				request.Parameters.Env,
				&actions_proto.VQLEnv{
					Key: k, Value: value_str,
				})
		}
	}

	principal := vql_subsystem.GetPrincipal(scope)
	result := &flows_proto.ArtifactCollectorResponse{Request: request}

	repository, err := artifacts.GetGlobalRepository(config_obj)
	if err != nil {
		scope.Log("collect_client: %v", err)
		return vfilter.Null{}
	}

	flow_id, err := services.GetLauncher().ScheduleArtifactCollection(
		ctx, config_obj, principal, repository, request)
	if err != nil {
		scope.Log("collect_client: %v", err)
		return vfilter.Null{}
	}

	// Notify the client about it.
	err = services.NotifyListener(config_obj, arg.ClientId)
	if err != nil {
		scope.Log("collect_client: %v", err)
		return vfilter.Null{}
	}

	result.FlowId = flow_id
	return result
}

func (self ScheduleCollectionFunction) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "collect_client",
		Doc:     "Launch an artifact collection against a client.",
		ArgType: type_map.AddType(scope, &ScheduleCollectionFunctionArg{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&ScheduleCollectionFunction{})
}
