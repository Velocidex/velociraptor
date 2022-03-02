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
	"www.velocidex.com/golang/velociraptor/artifacts"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/tools"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type ScheduleCollectionFunctionArg struct {
	ClientId     string      `vfilter:"required,field=client_id,doc=The client id to schedule a collection on"`
	Artifacts    []string    `vfilter:"required,field=artifacts,doc=A list of artifacts to collect"`
	Env          vfilter.Any `vfilter:"optional,field=env,doc=Parameters to apply to the artifact (an alternative to a full spec)"`
	Spec         vfilter.Any `vfilter:"optional,field=spec,doc=Parameters to apply to the artifacts"`
	Timeout      uint64      `vfilter:"optional,field=timeout,doc=Set query timeout (default 10 min)"`
	OpsPerSecond float64     `vfilter:"optional,field=ops_per_sec,doc=Set query ops_per_sec value"`
	CpuLimit     float64     `vfilter:"optional,field=cpu_limit,doc=Set query cpu_limit value"`
	IopsLimit    float64     `vfilter:"optional,field=iops_limit,doc=Set query iops_limit value"`
	MaxRows      uint64      `vfilter:"optional,field=max_rows,doc=Max number of rows to fetch"`
	MaxBytes     uint64      `vfilter:"optional,field=max_bytes,doc=Max number of bytes to upload"`
	Urgent       bool        `vfilter:"optional,field=urgent,doc=Set the collection as urgent - skips other queues collections on the client."`
}

type ScheduleCollectionFunction struct{}

func (self *ScheduleCollectionFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &ScheduleCollectionFunctionArg{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("collect_client: %s", err.Error())
		return vfilter.Null{}
	}

	if len(arg.Artifacts) == 0 {
		scope.Log("collect_client: no artifacts to collect!")
		return vfilter.Null{}
	}

	// Scheduling artifacts on the server requires higher
	// permissions.
	var permission acls.ACL_PERMISSION
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

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("collect_client: Command can only run on the server")
		return vfilter.Null{}
	}

	manager, err := services.GetRepositoryManager()
	if err != nil {
		scope.Log("collect_client: Command can only run on the server")
		return vfilter.Null{}
	}
	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		scope.Log("collect_client: Command can only run on the server")
		return vfilter.Null{}
	}

	request := &flows_proto.ArtifactCollectorArgs{
		ClientId:       arg.ClientId,
		Artifacts:      arg.Artifacts,
		Creator:        vql_subsystem.GetPrincipal(scope),
		OpsPerSecond:   float32(arg.OpsPerSecond),
		CpuLimit:       float32(arg.CpuLimit),
		IopsLimit:      float32(arg.IopsLimit),
		Timeout:        arg.Timeout,
		MaxRows:        arg.MaxRows,
		MaxUploadBytes: arg.MaxBytes,
		Urgent:         arg.Urgent,
	}

	if arg.Spec == nil {
		spec := ordereddict.NewDict()
		if arg.Env != nil {
			for _, name := range arg.Artifacts {
				spec.Set(name, arg.Env)
			}
		}
		arg.Spec = spec
	}

	err = tools.AddSpecProtobuf(config_obj, repository, scope,
		arg.Spec, request)
	if err != nil {
		scope.Log("collect_client: %v", err)
		return vfilter.Null{}
	}

	result := &flows_proto.ArtifactCollectorResponse{Request: request}
	acl_manager, ok := artifacts.GetACLManager(scope)
	if !ok {
		acl_manager = vql_subsystem.NullACLManager{}
	}

	launcher, err := services.GetLauncher()
	if err != nil {
		return vfilter.Null{}
	}

	flow_id, err := launcher.ScheduleArtifactCollection(
		ctx, config_obj, acl_manager, repository, request,
		func() {
			// Notify the client about it.
			notifier := services.GetNotifier()
			if notifier != nil {
				notifier.NotifyListener(
					config_obj, arg.ClientId, "collect_client")
			}
		})
	if err != nil {
		scope.Log("collect_client: %v", err)
		return vfilter.Null{}
	}

	result.FlowId = flow_id
	return json.ConvertProtoToOrderedDict(result)
}

func (self ScheduleCollectionFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "collect_client",
		Doc:     "Launch an artifact collection against a client.",
		ArgType: type_map.AddType(scope, &ScheduleCollectionFunctionArg{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&ScheduleCollectionFunction{})
}
