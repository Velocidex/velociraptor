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
	"www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/flows"
	"www.velocidex.com/golang/velociraptor/paths"
	artifact_paths "www.velocidex.com/golang/velociraptor/paths/artifacts"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type UploadsPluginsArgs struct {
	ClientId string `vfilter:"optional,field=client_id,doc=The client id to extract"`
	FlowId   string `vfilter:"optional,field=flow_id,doc=A flow ID (client or server artifacts)"`
}

type UploadsPlugins struct{}

func (self UploadsPlugins) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("uploads: %s", err)
			return
		}

		arg := &UploadsPluginsArgs{}

		config_obj, ok := artifacts.GetServerConfig(scope)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		// Allow the plugin args to override the environment scope.
		err = vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("uploads: %v", err)
			return
		}

		flow_path_manager := paths.NewFlowPathManager(arg.ClientId, arg.FlowId)
		row_chan, err := file_store.GetTimeRange(ctx, config_obj,
			flow_path_manager.UploadMetadata(), 0, 0)
		if err != nil {
			scope.Log("uploads: %v", err)
			return
		}

		for row := range row_chan {
			select {
			case <-ctx.Done():
				return
			case output_chan <- row:
			}
		}
	}()

	return output_chan
}

func (self UploadsPlugins) Info(
	scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "uploads",
		Doc:     "Retrieve information about a flow's uploads.",
		ArgType: type_map.AddType(scope, &UploadsPluginsArgs{}),
	}
}

type SourcePluginArgs struct {
	ClientId  string `vfilter:"optional,field=client_id,doc=The client id to extract"`
	DayName   string `vfilter:"optional,field=day_name,doc=Only extract this day's Monitoring logs (deprecated)"`
	StartTime int64  `vfilter:"optional,field=start_time,doc=Start return events from this date (for event sources)"`
	EndTime   int64  `vfilter:"optional,field=end_time,doc=Stop end events reach this time (event sources)."`
	FlowId    string `vfilter:"optional,field=flow_id,doc=A flow ID (client or server artifacts)"`
	HuntId    string `vfilter:"optional,field=hunt_id,doc=Retrieve sources from this hunt (combines all results from all clients)"`
	Artifact  string `vfilter:"optional,field=artifact,doc=The name of the artifact collection to fetch"`
	Source    string `vfilter:"optional,field=source,doc=An optional named source within the artifact"`
	Mode      string `vfilter:"optional,field=mode,doc=HUNT or CLIENT mode can be empty"`
}

type SourcePlugin struct{}

func (self SourcePlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("uploads: %s", err)
			return
		}

		arg := &SourcePluginArgs{}
		config_obj, ok := artifacts.GetServerConfig(scope)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		// This plugin will take parameters from environment
		// parameters. This allows its use to be more concise in
		// reports etc where many parameters can be inferred from
		// context.
		ParseSourceArgsFromScope(arg, scope)

		// Allow the plugin args to override the environment scope.
		err = vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("source: %v", err)
			return
		}

		// Hunt mode is just a proxy for the hunt_results()
		// plugin.
		if arg.HuntId != "" {
			args := ordereddict.NewDict().
				Set("hunt_id", arg.HuntId).
				Set("artifact", arg.Artifact).
				Set("source", arg.Source)

			// Just delegate to the hunt_results() plugin.
			plugin := &HuntResultsPlugin{}
			for row := range plugin.Call(ctx, scope, args) {
				select {
				case <-ctx.Done():
					return
				case output_chan <- row:
				}
			}
			return
		}

		if arg.Source != "" {
			arg.Artifact = arg.Artifact + "/" + arg.Source
			arg.Source = ""
		}

		path_manager := artifact_paths.NewArtifactPathManager(
			config_obj, arg.ClientId, arg.FlowId, arg.Artifact)

		row_chan, err := file_store.GetTimeRange(
			ctx, config_obj, path_manager, arg.StartTime, arg.EndTime)
		if err != nil {
			scope.Log("source: %v", err)
			return
		}

		for row := range row_chan {
			select {
			case <-ctx.Done():
				return
			case output_chan <- row:
			}
		}
	}()

	return output_chan
}

func (self SourcePlugin) Info(
	scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "source",
		Doc:     "Retrieve rows from an artifact's source.",
		ArgType: type_map.AddType(scope, &SourcePluginArgs{}),
	}
}

// Override SourcePluginArgs from the scope.
func ParseSourceArgsFromScope(arg *SourcePluginArgs, scope *vfilter.Scope) {
	client_id, pres := scope.Resolve("ClientId")
	if pres {
		arg.ClientId, _ = client_id.(string)
	}

	start_time, pres := scope.Resolve("StartTime")
	if pres {
		arg.StartTime, _ = start_time.(int64)
	}

	end_time, pres := scope.Resolve("EndTime")
	if pres {
		arg.EndTime, _ = end_time.(int64)
	}

	flow_id, pres := scope.Resolve("FlowId")
	if pres {
		arg.FlowId, _ = flow_id.(string)
	}

	hunt_id, pres := scope.Resolve("HuntId")
	if pres {
		arg.HuntId, _ = hunt_id.(string)
	}

	artifact_name, pres := scope.Resolve("ArtifactName")
	if pres {
		arg.Artifact = artifact_name.(string)
	}

	mode, pres := scope.Resolve("ReportMode")
	if pres {
		arg.Mode = mode.(string)
	}
}

type FlowResultsPluginArgs struct {
	Artifact string `vfilter:"optional,field=artifact,doc=The artifact to retrieve"`
	Source   string `vfilter:"optional,field=source,doc=An optional source within the artifact."`
	FlowId   string `vfilter:"required,field=flow_id,doc=The hunt id to read."`
	ClientId string `vfilter:"required,field=client_id,doc=The client id to extract"`
}

type FlowResultsPlugin struct{}

func (self FlowResultsPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	go func() {
		defer close(output_chan)

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("flow_results: %s", err)
			return
		}

		arg := &FlowResultsPluginArgs{}
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
		// the flow.
		if arg.Artifact == "" {
			flow, err := flows.GetFlowDetails(config_obj, arg.ClientId, arg.FlowId)
			if err != nil {
				scope.Log("flow_results: %v", err)
				return
			}

			if flow.Context != nil && flow.Context.Request != nil {
				requested_artifacts := flow.Context.Request.Artifacts
				if len(requested_artifacts) == 0 {
					scope.Log("flow_results: no artifacts in hunt")
					return
				}
				arg.Artifact = requested_artifacts[0]
			}
		}

		if arg.Source != "" {
			arg.Artifact = arg.Artifact + "/" + arg.Source
			arg.Source = ""
		}

		path_manager := artifact_paths.NewArtifactPathManager(
			config_obj, arg.ClientId, arg.FlowId, arg.Artifact)
		row_chan, err := file_store.GetTimeRange(
			ctx, config_obj, path_manager, 0, 0)
		if err != nil {
			scope.Log("source: %v", err)
			return
		}

		for row := range row_chan {
			select {
			case <-ctx.Done():
				return
			case output_chan <- row:
			}
		}
	}()

	return output_chan
}

func (self FlowResultsPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "flow_results",
		Doc:     "Retrieve the results of a flow.",
		ArgType: type_map.AddType(scope, &FlowResultsPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&SourcePlugin{})
	vql_subsystem.RegisterPlugin(&UploadsPlugins{})
	vql_subsystem.RegisterPlugin(&FlowResultsPlugin{})
}
