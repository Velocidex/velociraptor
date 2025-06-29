/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

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

package flows

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vql/tools/collector"
	vql_utils "www.velocidex.com/golang/velociraptor/vql/utils"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type ScheduleCollectionFunctionArg struct {
	ClientId     string            `vfilter:"required,field=client_id,doc=The client id to schedule a collection on"`
	Artifacts    []string          `vfilter:"required,field=artifacts,doc=A list of artifacts to collect"`
	Env          *ordereddict.Dict `vfilter:"optional,field=env,doc=Parameters to apply to the artifact (an alternative to a full spec)"`
	Spec         *ordereddict.Dict `vfilter:"optional,field=spec,doc=Parameters to apply to the artifacts"`
	Timeout      uint64            `vfilter:"optional,field=timeout,doc=Set query timeout (default 10 min)"`
	OpsPerSecond float64           `vfilter:"optional,field=ops_per_sec,doc=Set query ops_per_sec value"`
	CpuLimit     float64           `vfilter:"optional,field=cpu_limit,doc=Set query cpu_limit value"`
	IopsLimit    float64           `vfilter:"optional,field=iops_limit,doc=Set query iops_limit value"`
	MaxRows      uint64            `vfilter:"optional,field=max_rows,doc=Max number of rows to fetch"`
	MaxBytes     uint64            `vfilter:"optional,field=max_bytes,doc=Max number of bytes to upload"`
	Urgent       bool              `vfilter:"optional,field=urgent,doc=Set the collection as urgent - skips other queues collections on the client."`
	OrgId        string            `vfilter:"optional,field=org_id,doc=If set the collection will be started in the specified org."`
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

	// If a full spec is provided we dont need to provide the
	// artifacts again.
	if arg.Spec != nil && len(arg.Artifacts) == 0 {
		arg.Artifacts = arg.Spec.Keys()
	}

	if len(arg.Artifacts) == 0 {
		scope.Log("collect_client: no artifacts to collect!")
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("collect_client: %v", err)
		return vfilter.Null{}
	}

	// NOTE: Permission check is already made by
	// ScheduleArtifactCollection(). It is more complex as it depends
	// on permissions like:
	// COLLECT_CLIENT for clients
	// COLLECT_SERVER for server
	// COLLECT_BASIC for artifacts with the basic metadata set

	acl_manager, ok := artifacts.GetACLManager(scope)
	if !ok {
		acl_manager = acl_managers.NullACLManager{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("collect_client: %v", err)
		return vfilter.Null{}
	}

	// If we are required to switch orgs do so now.
	if arg.OrgId != "" {
		org_manager, err := services.GetOrgManager()
		if err != nil {
			scope.Log("collect_client: %v", err)
			return vfilter.Null{}
		}

		// If an org is specied we use the config obj from the org.
		config_obj, err = org_manager.GetOrgConfig(arg.OrgId)
		if err != nil {
			scope.Log("collect_client: %v", err)
			return vfilter.Null{}
		}

		// Switch the ACL manager into the required org
		org_acl_manager, ok := acl_manager.(vql_subsystem.OrgACLManager)
		if ok {
			org_acl_manager.SwitchDefaultOrg(config_obj)
		}
	}

	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		scope.Log("collect_client: %v", err)
		return vfilter.Null{}
	}

	err = client_info_manager.ValidateClientId(arg.ClientId)
	if err != nil {
		scope.Log("collect_client: %v", err)
		return vfilter.Null{}
	}

	repository, err := vql_utils.GetRepository(scope)
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

	err = collector.AddSpecProtobuf(ctx, config_obj, repository, scope,
		arg.Spec, request)
	if err != nil {
		scope.Log("collect_client: %v", err)
		return vfilter.Null{}
	}

	result := &flows_proto.ArtifactCollectorResponse{Request: request}

	launcher, err := services.GetLauncher(config_obj)
	if err != nil {
		return vfilter.Null{}
	}

	// ScheduleArtifactCollection already checks permissions.
	flow_id, err := launcher.ScheduleArtifactCollection(
		ctx, config_obj, acl_manager, repository, request,
		func() {
			// Notify the client about it.
			notifier, err := services.GetNotifier(config_obj)
			if err == nil {
				_ = notifier.NotifyListener(ctx,
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
		Metadata: vql.VQLMetadata().Permissions(
			acls.COLLECT_CLIENT, acls.COLLECT_SERVER, acls.COLLECT_BASIC).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&ScheduleCollectionFunction{})
}
