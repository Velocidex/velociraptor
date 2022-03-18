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
package flows

import (
	"context"
	"errors"
	"fmt"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/flows"
	"www.velocidex.com/golang/velociraptor/paths"
	artifact_paths "www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/server/hunts"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

// A one stop shop plugin for retrieving result sets collected from
// various places. Depending on the options used, the results come
// from different places in the filestore.
type SourcePluginArgs struct {
	// Collected artifacts from clients should specify the client
	// id and flow id as well as the artifact and source.
	ClientId string `vfilter:"optional,field=client_id,doc=The client id to extract"`
	FlowId   string `vfilter:"optional,field=flow_id,doc=A flow ID (client or server artifacts)"`

	// Specifying the hunt id will retrieve all rows in this hunt
	// (from all clients). You still need to specify the artifact
	// name.
	HuntId string `vfilter:"optional,field=hunt_id,doc=Retrieve sources from this hunt (combines all results from all clients)"`

	// Artifacts are specified by name and source. Name may
	// include the source following the artifact name by a slash -
	// e.g. Custom.Artifact/SourceName.
	Artifact string `vfilter:"optional,field=artifact,doc=The name of the artifact collection to fetch"`
	Source   string `vfilter:"optional,field=source,doc=An optional named source within the artifact"`

	// If the artifact name specifies an event artifact, you may
	// also specify start and end times to return.
	StartTime vfilter.Any `vfilter:"optional,field=start_time,doc=Start return events from this date (for event sources)"`
	EndTime   vfilter.Any `vfilter:"optional,field=end_time,doc=Stop end events reach this time (event sources)."`

	// A source may specify a notebook cell to read from - this
	// allows post processing in multiple stages - one query
	// reduces the data into a result set and subsequent queries
	// operate on that reduced set.
	NotebookId        string `vfilter:"optional,field=notebook_id,doc=The notebook to read from (should also include cell id)"`
	NotebookCellId    string `vfilter:"optional,field=notebook_cell_id,doc=The notebook cell read from (should also include notebook id)"`
	NotebookCellTable int64  `vfilter:"optional,field=notebook_cell_table,doc=A notebook cell can have multiple tables.)"`

	StartRow int64 `vfilter:"optional,field=start_row,doc=Start reading the result set from this row"`
	Limit    int64 `vfilter:"optional,field=count,doc=Maximum number of clients to fetch (default unlimited)'"`
}

type SourcePlugin struct{}

func (self SourcePlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
	if err != nil {
		scope.Log("uploads: %s", err)
		close(output_chan)
		return output_chan
	}

	arg := &SourcePluginArgs{}
	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
		close(output_chan)
		return output_chan
	}

	// This plugin will take parameters from environment
	// parameters. This allows its use to be more concise in
	// reports etc where many parameters can be inferred from
	// context.
	ParseSourceArgsFromScope(arg, scope)

	// Allow the plugin args to override the environment scope.
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("source: %v", err)
		close(output_chan)
		return output_chan
	}

	// Hunt mode is just a proxy for the hunt_results()
	// plugin.
	if arg.HuntId != "" {
		new_args := ordereddict.NewDict().
			Set("hunt_id", arg.HuntId).
			Set("artifact", arg.Artifact).
			Set("source", arg.Source)

		// Just delegate to the hunt_results() plugin.
		return hunts.HuntResultsPlugin{}.Call(ctx, scope, new_args)
	}

	// Event artifacts just proxy for the monitoring plugin.
	if arg.Artifact != "" {
		ok, _ := isArtifactEvent(config_obj, arg)
		if ok {
			// Just delegate directly to the monitoring plugin.
			return MonitoringPlugin{}.Call(ctx, scope, args)
		}
	}

	go func() {
		defer close(output_chan)

		// Depending on the parameters, we need to read from
		// different places.
		result_set_reader, err := getResultSetReader(ctx, config_obj, arg)
		if err != nil {
			scope.Log("source: %v", err)
			return
		}

		if arg.StartRow > 0 {
			err = result_set_reader.SeekToRow(arg.StartRow)
			if err != nil {
				scope.Log("source: %v", err)
				return
			}
		}

		count := int64(0)
		for row := range result_set_reader.Rows(ctx) {
			if arg.Limit > 0 && count >= arg.Limit {
				return
			}
			select {
			case <-ctx.Done():
				return
			case output_chan <- row:
				count++
			}
		}
	}()

	return output_chan
}

func (self SourcePlugin) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "source",
		Doc:     "Retrieve rows from stored result sets. This is a one stop show for retrieving stored result set for post processing.",
		ArgType: type_map.AddType(scope, &SourcePluginArgs{}),
	}
}

// Figure out if the artifact is an event artifact based on its
// definition.
func isArtifactEvent(
	config_obj *config_proto.Config,
	arg *SourcePluginArgs) (bool, error) {

	manager, err := services.GetRepositoryManager()
	if err != nil {
		return false, err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return false, err
	}

	artifact_definition, pres := repository.Get(config_obj, arg.Artifact)
	if !pres {
		return false, fmt.Errorf("Artifact %v not known", arg.Artifact)
	}

	switch artifact_definition.Type {
	case "client_event":
		if arg.ClientId == "" {
			return false, fmt.Errorf(
				"Artifact %v is a client event artifact, "+
					"therefore a client id is required.",
				artifact_definition.Name)
		}
		return true, nil

	case "server_event":
		return true, nil

	default:
		return false, nil
	}
}

func getResultSetReader(
	ctx context.Context,
	config_obj *config_proto.Config,
	arg *SourcePluginArgs) (result_sets.ResultSetReader, error) {

	file_store_factory := file_store.GetFileStore(config_obj)

	// Is it a notebook?
	if arg.NotebookId != "" && arg.NotebookCellId != "" {
		table := arg.NotebookCellTable
		if table == 0 {
			table = 1
		}
		path_manager := paths.NewNotebookPathManager(
			arg.NotebookId).Cell(arg.NotebookCellId).QueryStorage(table)

		return result_sets.NewResultSetReader(
			file_store_factory, path_manager.Path())
	}

	if arg.Artifact != "" {
		if arg.Source != "" {
			arg.Artifact = arg.Artifact + "/" + arg.Source
			arg.Source = ""
		}

		is_event, err := isArtifactEvent(config_obj, arg)
		if err != nil {
			return nil, err
		}

		// Event result sets can be sliced by time ranges.
		if is_event {
			return nil, errors.New("source plugin can not be used for event queries.")
		}

		// Must specify a client id and flow id for regular
		// collections.
		if arg.FlowId == "" || arg.ClientId == "" {
			return nil, errors.New("source: client_id and flow_id should " +
				"be specified for non event artifacts.")
		}

		path_manager, err := artifact_paths.NewArtifactPathManager(
			config_obj, arg.ClientId, arg.FlowId, arg.Artifact)
		if err != nil {
			return nil, err
		}

		return result_sets.NewResultSetReader(
			file_store_factory, path_manager.Path())

	}

	return nil, errors.New(
		"source: One of artifact, flow_id, hunt_id, notebook_id should be specified.")
}

// Override SourcePluginArgs from the scope.
func ParseSourceArgsFromScope(arg *SourcePluginArgs, scope vfilter.Scope) {
	client_id, pres := scope.Resolve("ClientId")
	if pres {
		arg.ClientId, _ = client_id.(string)
	}

	flow_id, pres := scope.Resolve("FlowId")
	if pres {
		arg.FlowId, _ = flow_id.(string)
	}

	artifact_name, pres := scope.Resolve("ArtifactName")
	if pres {
		artifact, ok := artifact_name.(string)
		if ok {
			arg.Artifact = artifact
		}
	}

	start_time, pres := scope.Resolve("StartTime")
	if pres {
		arg.StartTime = start_time
	}

	end_time, pres := scope.Resolve("EndTime")
	if pres {
		arg.EndTime = end_time
	}

	notebook_id, pres := scope.Resolve("NotebookId")
	if pres {
		arg.NotebookId, _ = notebook_id.(string)
	}
	notebook_cell_id, pres := scope.Resolve("NotebookCellId")
	if pres {
		arg.NotebookCellId, _ = notebook_cell_id.(string)
	}

	notebook_cell_table, pres := scope.Resolve("NotebookCellTable")
	if pres {
		arg.NotebookCellTable, _ = notebook_cell_table.(int64)
	}

	start_row, pres := scope.Resolve("StartRow")
	if pres {
		arg.StartRow, _ = start_row.(int64)
	}

	limit, pres := scope.Resolve("Limit")
	if pres {
		arg.Limit, _ = limit.(int64)
	}

	hunt_id, pres := scope.Resolve("HuntId")
	if pres {
		arg.HuntId, _ = hunt_id.(string)
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
	scope vfilter.Scope,
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

		path_manager, err := artifact_paths.NewArtifactPathManager(
			config_obj, arg.ClientId, arg.FlowId, arg.Artifact)
		if err != nil {
			scope.Log("source: %v", err)
			return
		}

		file_store_factory := file_store.GetFileStore(config_obj)
		rs_reader, err := result_sets.NewResultSetReader(
			file_store_factory, path_manager.Path())
		if err != nil {
			scope.Log("source: %v", err)
			return
		}

		for row := range rs_reader.Rows(ctx) {
			select {
			case <-ctx.Done():
				return
			case output_chan <- row:
			}
		}
	}()

	return output_chan
}

func (self FlowResultsPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "flow_results",
		Doc:     "Retrieve the results of a flow.",
		ArgType: type_map.AddType(scope, &FlowResultsPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&SourcePlugin{})
	vql_subsystem.RegisterPlugin(&FlowResultsPlugin{})
}
