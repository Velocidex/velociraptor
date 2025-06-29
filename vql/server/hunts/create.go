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
package hunts

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/velociraptor/vql/tools/collector"
	vql_utils "www.velocidex.com/golang/velociraptor/vql/utils"
	"www.velocidex.com/golang/vfilter/arg_parser"

	"www.velocidex.com/golang/vfilter"
)

type ScheduleHuntFunctionArg struct {
	Description   string           `vfilter:"optional,field=description,doc=Description of the hunt"`
	Artifacts     []string         `vfilter:"required,field=artifacts,doc=A list of artifacts to collect"`
	Expires       vfilter.LazyExpr `vfilter:"optional,field=expires,doc=A time for expiry (e.g. now() + 1800)"`
	Spec          vfilter.Any      `vfilter:"optional,field=spec,doc=Parameters to apply to the artifacts"`
	Timeout       uint64           `vfilter:"optional,field=timeout,doc=Set query timeout (default 10 min)"`
	OpsPerSecond  float64          `vfilter:"optional,field=ops_per_sec,doc=Set query ops_per_sec value"`
	CpuLimit      float64          `vfilter:"optional,field=cpu_limit,doc=Set query ops_per_sec value"`
	IopsLimit     float64          `vfilter:"optional,field=iops_limit,doc=Set query ops_per_sec value"`
	MaxRows       uint64           `vfilter:"optional,field=max_rows,doc=Max number of rows to fetch"`
	MaxBytes      uint64           `vfilter:"optional,field=max_bytes,doc=Max number of bytes to upload"`
	Pause         bool             `vfilter:"optional,field=pause,doc=If specified the new hunt will be in the paused state"`
	IncludeLabels []string         `vfilter:"optional,field=include_labels,doc=If specified only include these labels"`
	ExcludeLabels []string         `vfilter:"optional,field=exclude_labels,doc=If specified exclude these labels"`
	OS            string           `vfilter:"optional,field=os,doc=If specified target this OS"`
	OrgIds        []string         `vfilter:"optional,field=org_id,doc=If set the collection will be started in the specified orgs."`
}

type ScheduleHuntFunction struct{}

func (self *ScheduleHuntFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.START_HUNT)
	if err != nil {
		scope.Log("hunt: %v", err)
		return vfilter.Null{}
	}

	arg := &ScheduleHuntFunctionArg{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("hunt: %v", err)
		return vfilter.Null{}
	}

	var expires uint64
	if !utils.IsNil(arg.Expires) {
		expiry_time, err := functions.TimeFromAny(ctx, scope, arg.Expires.Reduce(ctx))
		if err != nil {
			scope.Log("hunt: expiry time invalid: %v", err)
			return vfilter.Null{}
		}

		// Check the time is in the future
		if expiry_time.Before(time.Now()) {
			scope.Log("hunt: expiry time %v in the past", expiry_time)
			return vfilter.Null{}
		}

		expires = uint64(expiry_time.UnixNano() / 1000)
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("hunt: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("hunt: Command can only run on the server")
		return vfilter.Null{}
	}

	// Only the org admin is allowed to launch on multiple orgs.
	if len(arg.OrgIds) > 0 {
		err := vql_subsystem.CheckAccess(scope, acls.ORG_ADMIN)
		if err != nil {
			scope.Log("hunt: %v", err)
			return vfilter.Null{}
		}

	} else {
		// Schedule on the current org
		arg.OrgIds = append(arg.OrgIds, utils.NormalizedOrgId(config_obj.OrgId))
	}

	repository, err := vql_utils.GetRepository(scope)
	if err != nil {
		scope.Log("hunt: %v", err)
		return vfilter.Null{}
	}

	request := &flows_proto.ArtifactCollectorArgs{
		Creator:        vql_subsystem.GetPrincipal(scope),
		Artifacts:      arg.Artifacts,
		OpsPerSecond:   float32(arg.OpsPerSecond),
		CpuLimit:       float32(arg.CpuLimit),
		IopsLimit:      float32(arg.IopsLimit),
		Timeout:        arg.Timeout,
		MaxRows:        arg.MaxRows,
		MaxUploadBytes: arg.MaxBytes,
	}

	principal := vql_subsystem.GetPrincipal(scope)
	err = collector.AddSpecProtobuf(ctx, config_obj, repository, scope,
		arg.Spec, request)
	if err != nil {
		scope.Log("hunt: %v", err)
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
		Expires:         expires,
		State:           state,
	}

	if len(arg.IncludeLabels) > 0 {
		if arg.OS != "" {
			scope.Log("hunt: Both OS and label conditions set, ignoring include label conditions")
		}

		hunt_request.Condition = &api_proto.HuntCondition{
			UnionField: &api_proto.HuntCondition_Labels{
				Labels: &api_proto.HuntLabelCondition{
					Label: arg.IncludeLabels,
				},
			},
		}

	}

	if len(arg.ExcludeLabels) > 0 {
		if hunt_request.Condition == nil {
			hunt_request.Condition = &api_proto.HuntCondition{}
		}
		hunt_request.Condition.ExcludedLabels = &api_proto.HuntLabelCondition{
			Label: arg.ExcludeLabels,
		}
	}

	switch arg.OS {
	case "":
		// Not specified
	case "linux":
		hunt_request.Condition = &api_proto.HuntCondition{
			UnionField: &api_proto.HuntCondition_Os{
				Os: &api_proto.HuntOsCondition{
					Os: api_proto.HuntOsCondition_LINUX,
				},
			},
		}
	case "windows":
		hunt_request.Condition = &api_proto.HuntCondition{
			UnionField: &api_proto.HuntCondition_Os{
				Os: &api_proto.HuntOsCondition{
					Os: api_proto.HuntOsCondition_WINDOWS,
				},
			},
		}

	case "darwin":
		hunt_request.Condition = &api_proto.HuntCondition{
			UnionField: &api_proto.HuntCondition_Os{
				Os: &api_proto.HuntOsCondition{
					Os: api_proto.HuntOsCondition_OSX,
				},
			},
		}

	default:
		scope.Log("hunt: OS condition invalid %v (should be linux, windows, darwin)", arg.OS)
		return vfilter.Null{}
	}

	org_manager, err := services.GetOrgManager()
	if err != nil {
		scope.Log("hunt: %v", err)
		return vfilter.Null{}
	}

	var orgs_we_scheduled []string

	var new_hunt *api_proto.Hunt

	// Schedule the hunt on all the relevant orgs.
	for _, org_id := range arg.OrgIds {
		org_config_obj, err := org_manager.GetOrgConfig(org_id)
		if err != nil {
			scope.Log("hunt: %v", err)
			continue
		}

		// Make sure the user is allowed to collect in that org
		err = vql_subsystem.CheckAccessInOrg(scope, org_id, acls.START_HUNT)
		if err != nil {
			scope.Log("hunt: %v", err)
			continue
		}

		// Run the hunt in the ACL context of the caller.
		acl_manager := acl_managers.NewServerACLManager(
			org_config_obj, vql_subsystem.GetPrincipal(scope))

		hunt_dispatcher, err := services.GetHuntDispatcher(org_config_obj)
		if err != nil {
			scope.Log("hunt: %v", err)
			continue
		}

		// Only mark the org ids in the root org version of the hunt.
		org_hunt_request := hunt_request
		if utils.IsRootOrg(org_id) {
			org_hunt_request = proto.Clone(hunt_request).(*api_proto.Hunt)
			org_hunt_request.OrgIds = arg.OrgIds
		}

		new_hunt, err = hunt_dispatcher.CreateHunt(
			ctx, org_config_obj, acl_manager, org_hunt_request)
		if err != nil {
			scope.Log("hunt: %v", err)
			continue
		}

		orgs_we_scheduled = append(orgs_we_scheduled, org_id)

		// The first hunt will create an Id then subsequent hunts will
		// reuse same ID.
		hunt_request.HuntId = new_hunt.HuntId
	}

	if new_hunt == nil {
		return vfilter.Null{}
	}

	err = services.LogAudit(ctx,
		config_obj, principal, "CreateHunt",
		ordereddict.NewDict().
			Set("hunt_id", new_hunt.HuntId).
			Set("details", vfilter.RowToDict(ctx, scope, arg)).
			Set("orgs", orgs_we_scheduled))
	if err != nil {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Error("<red>CreateHunt</> %v %v", principal, new_hunt.HuntId)
	}

	return ordereddict.NewDict().
		Set("HuntId", new_hunt.HuntId).
		Set("Request", new_hunt)
}

func (self ScheduleHuntFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "hunt",
		Doc:      "Launch an artifact collection against a client.",
		ArgType:  type_map.AddType(scope, &ScheduleHuntFunctionArg{}),
		Metadata: vql.VQLMetadata().Permissions(acls.START_HUNT, acls.ORG_ADMIN).Build(),
	}
}

type AddToHuntFunctionArg struct {
	ClientId string `vfilter:"required,field=client_id"`
	HuntId   string `vfilter:"required,field=hunt_id"`
	FlowId   string `vfilter:"optional,field=flow_id,doc=If a flow id is specified we do not create a new flow, but instead add this flow_id to the hunt."`
	Relaunch bool   `vfilter:"optional,field=relaunch,doc=If specified we relaunch the hunt on this client again."`
}

type AddToHuntFunction struct{}

func (self *AddToHuntFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.START_HUNT)
	if err != nil {
		scope.Log("hunt_add: %v", err)
		return vfilter.Null{}
	}

	arg := &AddToHuntFunctionArg{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("hunt_add: %v", err)
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("hunt_add: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("hunt_add: Command can only run on the server")
		return vfilter.Null{}
	}

	journal, _ := services.GetJournal(config_obj)
	if journal == nil {
		return vfilter.Null{}
	}

	// Relaunch the collection.
	if arg.Relaunch {
		hunt_dispatcher, err := services.GetHuntDispatcher(config_obj)
		if err != nil {
			return vfilter.Null{}
		}

		hunt_obj, pres := hunt_dispatcher.GetHunt(ctx, arg.HuntId)
		if !pres || hunt_obj == nil ||
			hunt_obj.StartRequest == nil ||
			hunt_obj.StartRequest.CompiledCollectorArgs == nil {
			scope.Log("hunt_add: Hunt id not found %v", arg.HuntId)
			return vfilter.Null{}
		}

		launcher, err := services.GetLauncher(config_obj)
		if err != nil {
			return vfilter.Null{}
		}

		// Launch the collection against a client. We assume it is
		// already compiled because hunts always pre-compile their
		// artifacts.
		request := proto.Clone(hunt_obj.StartRequest).(*flows_proto.ArtifactCollectorArgs)
		request.ClientId = arg.ClientId

		// Generate a new flow id for each request
		request.FlowId = ""

		arg.FlowId, err = launcher.WriteArtifactCollectionRecord(
			ctx, config_obj, request, hunt_obj.StartRequest.CompiledCollectorArgs,
			func(task *crypto_proto.VeloMessage) {
				client_manager, err := services.GetClientInfoManager(config_obj)
				if err != nil {
					return
				}

				// Queue and notify the client about the new tasks
				_ = client_manager.QueueMessageForClient(
					ctx, arg.ClientId, task,
					services.NOTIFY_CLIENT, utils.BackgroundWriter)
			})
		if err != nil {
			scope.Log("hunt_add: %v", err)
			return vfilter.Null{}
		}

		err = hunt_dispatcher.MutateHunt(ctx, config_obj,
			&api_proto.HuntMutation{
				HuntId: arg.HuntId,
				Assignment: &api_proto.FlowAssignment{
					ClientId: arg.ClientId,
					FlowId:   arg.FlowId,
				}})
		if err != nil {
			scope.Log("hunt_add: %v", err)
			return vfilter.Null{}
		}

		return arg.FlowId
	}

	// Send this
	if arg.FlowId != "" {
		err = journal.PushRowsToArtifact(ctx, config_obj,
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
		err = journal.PushRowsToArtifact(ctx, config_obj,
			[]*ordereddict.Dict{ordereddict.NewDict().
				Set("HuntId", arg.HuntId).
				Set("ClientId", arg.ClientId).
				Set("Override", true)},
			"System.Hunt.Participation", arg.ClientId, "")
	}

	if err != nil {
		scope.Log("hunt_add: %v", err)
		return vfilter.Null{}
	}

	return arg.ClientId
}

func (self AddToHuntFunction) Info(scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "hunt_add",
		Doc:      "Assign a client to a hunt.",
		ArgType:  type_map.AddType(scope, &AddToHuntFunctionArg{}),
		Metadata: vql.VQLMetadata().Permissions(acls.START_HUNT).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&ScheduleHuntFunction{})
	vql_subsystem.RegisterFunction(&AddToHuntFunction{})
}
