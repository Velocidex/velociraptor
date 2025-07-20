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
// VQL plugins for running on the server.

package hunts

import (
	"context"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/json"
	artifact_paths "www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/hunt_dispatcher"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type HuntsPluginArgs struct {
	HuntId string `vfilter:"optional,field=hunt_id,doc=A hunt id to read, if not specified we list all of them."`
}

type HuntsPlugin struct{}

func (self HuntsPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "hunts", args)()

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("hunts: %s", err)
			return
		}

		arg := &HuntsPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("hunts: %v", err)
			return
		}

		err = services.RequireFrontend()
		if err != nil {
			scope.Log("hunts: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("hunts: Command can only run on the server")
			return
		}

		hunt_dispatcher, err := services.GetHuntDispatcher(config_obj)
		if err != nil {
			scope.Log("hunts: %v", err)
			return
		}

		// Show a specific hunt
		if arg.HuntId != "" {
			hunt_obj, pres := hunt_dispatcher.GetHunt(ctx, arg.HuntId)
			if pres {
				select {
				case <-ctx.Done():
					return
				case output_chan <- json.ConvertProtoToOrderedDict(hunt_obj):
				}
			}
			return
		}

		// Show all hunts.
		var hunts []*api_proto.Hunt

		err = hunt_dispatcher.ApplyFuncOnHunts(
			ctx, services.AllHunts,
			func(hunt *api_proto.Hunt) error {
				hunts = append(hunts, hunt)
				return nil
			})
		if err != nil {
			scope.Log("hunts: %v", err)
			return
		}

		for _, hunt_obj := range hunts {
			select {
			case <-ctx.Done():
				return
			case output_chan <- json.ConvertProtoToOrderedDict(hunt_obj):
			}
		}
	}()

	return output_chan
}

func (self HuntsPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "hunts",
		Doc:      "Retrieve the list of hunts.",
		ArgType:  type_map.AddType(scope, &HuntsPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.READ_RESULTS).Build(),
	}
}

type HuntResultsPluginArgs struct {
	Artifact string   `vfilter:"optional,field=artifact,doc=The artifact to retrieve"`
	Source   string   `vfilter:"optional,field=source,doc=An optional source within the artifact."`
	HuntId   string   `vfilter:"required,field=hunt_id,doc=The hunt id to read."`
	Brief    bool     `vfilter:"optional,field=brief,doc=If set we return less columns (deprecated)."`
	Orgs     []string `vfilter:"optional,field=orgs,doc=If set we combine results from all orgs."`
}

type HuntResultsPlugin struct{}

func (self HuntResultsPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "hunt_results", args)()

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("hunt_results: %s", err)
			return
		}

		arg := &HuntResultsPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("hunt_results: %v", err)
			return
		}

		err = services.RequireFrontend()
		if err != nil {
			scope.Log("hunt_results: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("hunt_results: Command can only run on the server")
			return
		}

		available_artifacts, err := self.GetAvailableArtifacts(ctx, config_obj, arg.HuntId)
		if err != nil {
			scope.Log("hunt_results: %v", err)
			return
		}

		if len(available_artifacts) == 0 {
			scope.Log("hunt_results: not artifacts available")
			return
		}

		// If no artifact is specified, get the first one from
		// the hunt.
		if arg.Artifact == "" {
			arg.Artifact = available_artifacts[0]
		} else if arg.Source != "" {
			arg.Artifact += "/" + arg.Source
		}

		if !utils.InString(available_artifacts, arg.Artifact) {
			scope.Log("hunt_results: artifact %v not available in hunt. "+
				"Available artifacts are %v",
				arg.Artifact, strings.Join(available_artifacts, ", "))
			return
		}

		if len(arg.Orgs) == 0 {
			arg.Orgs = append(arg.Orgs, config_obj.OrgId)
		}

		principal := vql_subsystem.GetPrincipal(scope)

		org_manager, err := services.GetOrgManager()
		if err != nil {
			return
		}

		for _, org_id := range arg.Orgs {
			org_config_obj, err := org_manager.GetOrgConfig(org_id)
			if err != nil {
				continue
			}

			// Make sure the principal has read access in this org.
			permissions := acls.READ_RESULTS
			perm, err := services.CheckAccess(
				org_config_obj, principal, permissions)
			if !perm || err != nil {
				continue
			}

			indexer, err := services.GetIndexer(org_config_obj)
			if err != nil {
				return
			}

			hunt_dispatcher, err := services.GetHuntDispatcher(org_config_obj)
			if err != nil {
				return
			}

			options := services.FlowSearchOptions{BasicInformation: true}
			flow_chan, _, err := hunt_dispatcher.GetFlows(
				ctx, org_config_obj, options, scope, arg.HuntId, 0)
			if err != nil {
				// If there are no flows in this hunt - it is not an
				// error it just means no results.
				return
			}

			for flow_details := range flow_chan {

				// Use the indexer for enriching with Fqdn
				fqdn := ""
				api_client, err := indexer.FastGetApiClient(ctx,
					org_config_obj, flow_details.Context.ClientId)
				if err == nil {
					if api_client.OsInfo != nil {
						fqdn = api_client.OsInfo.Fqdn
					}
				}

				artifact_name := arg.Artifact
				if arg.Source != "" {
					artifact_name += "/" + arg.Source
				}

				// Read individual flow's results.
				path_manager, err := artifact_paths.NewArtifactPathManager(
					ctx, org_config_obj,
					flow_details.Context.ClientId,
					flow_details.Context.SessionId,
					artifact_name)
				if err != nil {
					continue
				}

				file_store_factory := file_store.GetFileStore(org_config_obj)

				reader, err := result_sets.NewResultSetReader(
					file_store_factory, path_manager.Path())
				if err != nil {
					continue
				}

				// Read each result set and emit it
				// with some extra columns for
				// context.
				for row := range reader.Rows(ctx) {
					row.Set("FlowId", flow_details.Context.SessionId).
						Set("ClientId", flow_details.Context.ClientId).
						Set("_OrgId", org_id).
						Set("Fqdn", fqdn)

					select {
					case <-ctx.Done():
						return
					case output_chan <- row:
					}
				}
			}
		}
	}()

	return output_chan
}

func (self HuntResultsPlugin) GetAvailableArtifacts(
	ctx context.Context,
	config_obj *config_proto.Config, hunt_id string) ([]string, error) {

	hunt_dispatcher_service, err := services.GetHuntDispatcher(config_obj)
	if err != nil {
		return nil, err
	}

	hunt_obj, pres := hunt_dispatcher_service.GetHunt(ctx, hunt_id)
	if !pres {
		return nil, utils.Wrap(utils.NotFoundError, "Hunt not found")
	}

	hunt_dispatcher.FindCollectedArtifacts(ctx, config_obj, hunt_obj)

	var artifacts []string

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return nil, err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}

	for _, artifact := range hunt_obj.Artifacts {
		artifact_def, ok := repository.Get(ctx, config_obj, artifact)
		if ok {
			for _, source := range artifact_def.Sources {
				name := artifact_def.Name

				if source.Name != "" {
					name += "/" + source.Name
				}

				artifacts = append(artifacts, name)
			}
		}
	}

	return artifacts, nil
}

func (self HuntResultsPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "hunt_results",
		Doc:      "Retrieve the results of a hunt.",
		ArgType:  type_map.AddType(scope, &HuntResultsPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.READ_RESULTS).Build(),
	}
}

type HuntFlowsPluginArgs struct {
	HuntId           string `vfilter:"required,field=hunt_id,doc=The hunt id to inspect."`
	StartRow         int64  `vfilter:"optional,field=start_row,doc=The first row to show (used for paging)."`
	Limit            int64  `vfilter:"optional,field=limit,doc=Number of rows to show (used for paging)."`
	BasicInformation bool   `vfilter:"optional,field=basic_info,doc=If specified we only return basic information like flow id and client id."`
}

type HuntFlowsPlugin struct{}

func (self HuntFlowsPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "hunt_flows", args)()

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("hunt_flows: %s", err)
			return
		}

		arg := &HuntFlowsPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("hunt_flows: %v", err)
			return
		}

		err = services.RequireFrontend()
		if err != nil {
			scope.Log("hunt_flows: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("hunt_flows: Command can only run on the server")
			return
		}

		hunt_dispatcher, err := services.GetHuntDispatcher(config_obj)
		if err != nil {
			scope.Log("hunt_flows: %v", err)
			return
		}

		// Get full information about all the flows in the hunt.
		options := services.FlowSearchOptions{
			BasicInformation: arg.BasicInformation,
		}
		flow_chan, _, err := hunt_dispatcher.GetFlows(
			ctx, config_obj, options,
			scope, arg.HuntId, int(arg.StartRow))
		if err != nil {
			scope.Log("hunt_flows: %v", err)
			return
		}

		for flow_details := range flow_chan {
			if flow_details == nil || flow_details.Context == nil {
				continue
			}

			client_id := flow_details.Context.ClientId
			flow_id := flow_details.Context.SessionId

			result := ordereddict.NewDict().
				Set("HuntId", arg.HuntId).
				Set("ClientId", client_id).
				Set("FlowId", flow_id).
				Set("Flow", json.ConvertProtoToOrderedDict(
					flow_details.Context))

			select {
			case <-ctx.Done():
				return
			case output_chan <- result:
			}
		}
	}()

	return output_chan
}

func (self HuntFlowsPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "hunt_flows",
		Doc:      "Retrieve the flows launched by a hunt.",
		ArgType:  type_map.AddType(scope, &HuntFlowsPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.READ_RESULTS).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&HuntsPlugin{})
	vql_subsystem.RegisterPlugin(&HuntResultsPlugin{})
	vql_subsystem.RegisterPlugin(&HuntFlowsPlugin{})
}
