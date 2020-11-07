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

package server

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/api"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/result_sets"
	"www.velocidex.com/golang/velociraptor/flows"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	artifact_paths "www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/hunt_manager"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type HuntsPluginArgs struct{}

type HuntsPlugin struct{}

func (self HuntsPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
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
		err = vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("hunts: %v", err)
			return
		}

		config_obj, ok := artifacts.GetServerConfig(scope)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		db, err := datastore.GetDB(config_obj)
		if err != nil {
			scope.Log("Error: %v", err)
			return
		}

		hunt_path_manager := paths.NewHuntPathManager("")
		hunts, err := db.ListChildren(config_obj,
			hunt_path_manager.HuntDirectory().Path(), 0, 100)
		if err != nil {
			scope.Log("Error: %v", err)
			return
		}

		for _, hunt_urn := range hunts {
			hunt_obj := &api_proto.Hunt{}
			err = db.GetSubject(config_obj, hunt_urn, hunt_obj)
			if err != nil {
				continue
			}

			// Re-read the stats into the hunt object.
			hunt_path_manager := paths.NewHuntPathManager(hunt_obj.HuntId)
			hunt_stats := &api_proto.HuntStats{}
			err := db.GetSubject(config_obj,
				hunt_path_manager.Stats().Path(), hunt_stats)
			if err == nil {
				hunt_obj.Stats = hunt_stats
			}

			select {
			case <-ctx.Done():
				return
			case output_chan <- hunt_obj:
			}
		}
	}()

	return output_chan
}

func (self HuntsPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
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
	scope *vfilter.Scope,
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
		err = vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("hunt_results: %v", err)
			return
		}

		config_obj, ok := artifacts.GetServerConfig(scope)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		// If no artifact is specified, get the first one from
		// the hunt.
		if arg.Artifact == "" {
			db, err := datastore.GetDB(config_obj)
			if err != nil {
				scope.Log("hunt_results: %v", err)
				return
			}

			hunt_path_manager := paths.NewHuntPathManager(arg.HuntId)
			hunt_obj := &api_proto.Hunt{}
			err = db.GetSubject(config_obj,
				hunt_path_manager.Path(), hunt_obj)
			if err != nil {
				scope.Log("hunt_results: %v", err)
				return
			}

			flows.FindCollectedArtifacts(config_obj, hunt_obj)
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

		// Backwards compatibility.
		hunt_path_manager := paths.NewHuntPathManager(arg.HuntId).Clients()
		row_chan, err := file_store.GetTimeRange(ctx, config_obj,
			hunt_path_manager, 0, 0)
		if err != nil {
			return
		}

		// Read each file and emit it with some extra columns
		// for context.
		for row := range row_chan {
			participation_row := &hunt_manager.ParticipationRecord{}
			err := vfilter.ExtractArgs(scope, row, participation_row)
			if err != nil {
				continue
			}

			if participation_row.Participate {
				api_client, err := api.GetApiClient(
					config_obj, nil, participation_row.ClientId, false)
				if err != nil {
					continue
				}

				// Read individual flow's results.
				path_manager := artifact_paths.NewArtifactPathManager(
					config_obj,
					participation_row.ClientId,
					participation_row.FlowId,
					arg.Artifact)
				row_chan, err := file_store.GetTimeRange(
					ctx, config_obj, path_manager, 0, 0)
				if err != nil {
					continue
				}

				// Read each result set and emit it
				// with some extra columns for
				// context.
				for row := range row_chan {
					row.Set("FlowId", participation_row.FlowId).
						Set("ClientId", participation_row.ClientId)

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
		}
	}()

	return output_chan
}

func (self HuntResultsPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
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
	scope *vfilter.Scope,
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
		err = vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("hunt_flows: %v", err)
			return
		}

		config_obj, ok := artifacts.GetServerConfig(scope)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		hunt_path_manager := paths.NewHuntPathManager(arg.HuntId).Clients()
		rs_reader, err := result_sets.NewResultSetReader(config_obj, hunt_path_manager)
		if err != nil {
			scope.Log("Error %v: %v\n", err, hunt_path_manager.Path())
			return
		}
		defer rs_reader.Close()

		// Seek to the row we need.
		err = rs_reader.SeekToRow(int64(arg.StartRow))
		if err != nil {
			scope.Log("Error %v: %v\n", err, hunt_path_manager.Path())
			return
		}

		for row := range rs_reader.Rows(ctx) {
			participation_row := &hunt_manager.ParticipationRecord{}
			err := vfilter.ExtractArgs(scope, row, participation_row)
			if err != nil {
				return
			}

			result := ordereddict.NewDict().
				Set("HuntId", participation_row.HuntId).
				Set("ClientId", participation_row.ClientId).
				Set("FlowId", participation_row.FlowId).
				Set("Flow", vfilter.Null{})

			collection_context, err := flows.LoadCollectionContext(
				config_obj, participation_row.ClientId,
				participation_row.FlowId)
			if err == nil {
				result.Set("Flow",
					json.ConvertProtoToOrderedDict(collection_context))
			}

			select {
			case <-ctx.Done():
				return
			case output_chan <- result:
			}
		}
	}()

	return output_chan
}

func (self HuntFlowsPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
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
