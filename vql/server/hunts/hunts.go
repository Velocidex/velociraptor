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
// VQL plugins for running on the server.

package hunts

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	artifact_paths "www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/hunt_dispatcher"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type HuntsPluginArgs struct {
	HuntId string `vfilter:"optional,field=hunt_id,doc=A hunt id to read, if not specified we list all of them."`
	Offset uint64 `vfilter:"optional,field=offset,doc=Start offset."`
	Count  uint64 `vfilter:"optional,field=count,doc=Max number of results to return."`
}

type HuntsPlugin struct{}

func (self HuntsPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	go func() {
		defer close(output_chan)

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

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		count := arg.Count
		if count == 0 {
			count = 1000
		}

		hunt_dispatcher := services.GetHuntDispatcher()

		// Show a specific hunt
		if arg.HuntId != "" {
			hunt_obj, pres := hunt_dispatcher.GetHunt(arg.HuntId)
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
		hunts, err := hunt_dispatcher.ListHunts(
			ctx, config_obj, &api_proto.ListHuntsRequest{
				Count:  count,
				Offset: arg.Offset,
			})
		if err != nil {
			scope.Log("hunts: %v", err)
			return
		}

		for _, hunt_obj := range hunts.Items {
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
		Name:    "hunts",
		Doc:     "Retrieve the list of hunts.",
		ArgType: type_map.AddType(scope, &HuntsPluginArgs{}),
	}
}

type HuntResultsPluginArgs struct {
	Artifact string `vfilter:"optional,field=artifact,doc=The artifact to retrieve"`
	Source   string `vfilter:"optional,field=source,doc=An optional source within the artifact."`
	HuntId   string `vfilter:"required,field=hunt_id,doc=The hunt id to read."`
	Brief    bool   `vfilter:"optional,field=brief,doc=If set we return less columns."`
}

type HuntResultsPlugin struct{}

func (self HuntResultsPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

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

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		// If no artifact is specified, get the first one from
		// the hunt.
		if arg.Artifact == "" {
			hunt_dispatcher_service := services.GetHuntDispatcher()
			hunt_obj, pres := hunt_dispatcher_service.GetHunt(arg.HuntId)
			if !pres {
				return
			}

			hunt_dispatcher.FindCollectedArtifacts(config_obj, hunt_obj)
			if len(hunt_obj.Artifacts) == 0 {
				scope.Log("hunt_results: no artifacts in hunt")
				return
			}

			if arg.Source == "" {
				arg.Artifact, arg.Source = paths.SplitFullSourceName(
					hunt_obj.Artifacts[0])
			}

			// If the source is not specified find the
			// first named source from the artifact
			// definition.
			if arg.Source == "" {
				manager, err := services.GetRepositoryManager()
				if err != nil {
					scope.Log("hunt_results: %v", err)
					return
				}
				repo, err := manager.GetGlobalRepository(config_obj)
				if err == nil {
					artifact_def, ok := repo.Get(config_obj, arg.Artifact)
					if ok {
						for _, source := range artifact_def.Sources {
							if source.Name != "" {
								arg.Source = source.Name
								break
							}
						}
					}
				}
			}

		} else if arg.Source != "" {
			arg.Artifact += "/" + arg.Source
		}

		indexer, err := services.GetIndexer()
		if err != nil {
			return
		}

		hunt_dispatcher := services.GetHuntDispatcher()
		for flow_details := range hunt_dispatcher.GetFlows(
			ctx, config_obj, scope, arg.HuntId, 0) {

			api_client, err := indexer.FastGetApiClient(ctx,
				config_obj, flow_details.Context.ClientId)
			if err != nil {
				scope.Log("hunt_results: %v", err)
				continue
			}

			// Read individual flow's results.
			path_manager, err := artifact_paths.NewArtifactPathManager(
				config_obj,
				flow_details.Context.ClientId,
				flow_details.Context.SessionId,
				arg.Artifact)
			if err != nil {
				continue
			}

			file_store_factory := file_store.GetFileStore(config_obj)

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
					Set("ClientId", flow_details.Context.ClientId)

				if api_client.OsInfo != nil {
					row.Set("Fqdn", api_client.OsInfo.Fqdn)
				}
				select {
				case <-ctx.Done():
					return
				case output_chan <- row:
				}
			}
		}
	}()

	return output_chan
}

func (self HuntResultsPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "hunt_results",
		Doc:     "Retrieve the results of a hunt.",
		ArgType: type_map.AddType(scope, &HuntResultsPluginArgs{}),
	}
}

type HuntFlowsPluginArgs struct {
	HuntId   string `vfilter:"required,field=hunt_id,doc=The hunt id to inspect."`
	StartRow int64  `vfilter:"optional,field=start_row,doc=The first row to show (used for paging)."`
	Limit    int64  `vfilter:"optional,field=limit,doc=Number of rows to show (used for paging)."`
}

type HuntFlowsPlugin struct{}

func (self HuntFlowsPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	go func() {
		defer close(output_chan)

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

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		hunt_dispatcher := services.GetHuntDispatcher()
		for flow_details := range hunt_dispatcher.GetFlows(
			ctx, config_obj, scope, arg.HuntId, int(arg.StartRow)) {

			result := ordereddict.NewDict().
				Set("HuntId", arg.HuntId).
				Set("ClientId", flow_details.Context.ClientId).
				Set("FlowId", flow_details.Context.SessionId).
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
		Name:    "hunt_flows",
		Doc:     "Retrieve the flows launched by a hunt.",
		ArgType: type_map.AddType(scope, &HuntFlowsPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&HuntsPlugin{})
	vql_subsystem.RegisterPlugin(&HuntResultsPlugin{})
	vql_subsystem.RegisterPlugin(&HuntFlowsPlugin{})
}
