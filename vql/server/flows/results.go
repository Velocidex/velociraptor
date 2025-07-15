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
	"errors"
	"fmt"
	"io"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/paths"
	artifact_paths "www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/server/hunts"
	vql_utils "www.velocidex.com/golang/velociraptor/vql/utils"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type pluginMode int

const (
	MODE_UNSET pluginMode = iota
	MODE_FLOW_ARTIFACT
	MODE_HUNT_ARTIFACT
	MODE_EVENT_ARTIFACT
	MODE_NOTEBOOK
)

// A one stop shop plugin for retrieving result sets collected from
// various places. Depending on the options used, the results come
// from different places in the filestore. Because multiple sources
// can be specified at the same time there is a preference order:

// 1. If NotebookId is specified, then it is a notebook cell we read
//    from.
// 2. If a HuntId is specified we read from the hunt
// 3. If an event Artifact is specified we read from the monitoring
//    log for that artifact.
// 4. If a FlowId is specified then we read from the collection.

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
	NotebookId          string `vfilter:"optional,field=notebook_id,doc=The notebook to read from (should also include cell id)"`
	NotebookCellId      string `vfilter:"optional,field=notebook_cell_id,doc=The notebook cell read from (should also include notebook id)"`
	NotebookCellVersion string `vfilter:"optional,field=notebook_cell_version,doc=The notebook cell version to read from (should also include notebook id and notebook cell)"`
	NotebookCellTable   int64  `vfilter:"optional,field=notebook_cell_table,doc=A notebook cell can have multiple tables.)"`

	StartRow int64 `vfilter:"optional,field=start_row,doc=Start reading the result set from this row"`
	Limit    int64 `vfilter:"optional,field=count,doc=Maximum number of rows to fetch (default unlimited)"`

	OrgIds []string `vfilter:"optional,field=orgs,doc=Run the query over these orgs. If empty use the current org."`

	// The source plugin works by providing direct arguments **and**
	// deducing some arguments from the environment. Depending on
	// which argument is used the plugin does different things.
	//
	// We use the arguments provided directly to decide what mode we
	// are operating in:
	//
	// 1. If the Artifact is provided we are in artifact result
	//    reading mode.
	// 2. If the NotebookId is provided directly we are in notebook
	//    mode - we read the results from another notebook cell.
	mode pluginMode
}

func (self *SourcePluginArgs) DetermineMode(
	ctx context.Context, config_obj *config_proto.Config,
	scope vfilter.Scope, args *ordereddict.Dict) error {

	if args != nil {
		_, pres := args.Get("notebook_id")
		if pres {
			self.mode = MODE_NOTEBOOK
			return nil
		}
	}

	// This plugin will take parameters from environment
	// parameters. This allows its use to be more concise in
	// reports etc where many parameters can be inferred from
	// context.
	self.ParseSourceArgsFromScope(scope)

	if self.Artifact != "" {
		// Normalize the artifact name to include the source
		if self.Source != "" {
			self.Artifact = self.Artifact + "/" + self.Source
			self.Source = ""
		}

		// Is this a hunt result set?
		if self.HuntId != "" {
			self.mode = MODE_HUNT_ARTIFACT
			return nil
		}

		// Detect if the artifact is an event artifact
		is_event, client_id, err := self.isArtifactEvent(scope, ctx, config_obj)
		if err != nil {
			return err
		}

		if is_event {
			self.ClientId = client_id
			self.mode = MODE_EVENT_ARTIFACT
			return nil
		}

		// Must specify a client id and flow id for regular
		// collections.
		if self.FlowId == "" || self.ClientId == "" {
			return errors.New("source: client_id and flow_id should " +
				"be specified for non event artifacts.")
		}
		self.mode = MODE_FLOW_ARTIFACT
		return nil
	}

	return errors.New(
		"source: One of artifact, flow_id, hunt_id, notebook_id should be specified.")
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

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("uploads: %v", err)
		close(output_chan)
		return output_chan
	}

	arg := &SourcePluginArgs{}
	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("uploads: Command can only run on the server")
		close(output_chan)
		return output_chan
	}

	// Allow the plugin args to override the environment scope.
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("source: %v", err)
		close(output_chan)
		return output_chan
	}

	// Determine the mode based on the args passed.
	err = arg.DetermineMode(ctx, config_obj, scope, args)
	if err != nil {
		scope.Log("source: %v", err)
		close(output_chan)
		return output_chan
	}

	// Hunt mode is just a proxy for the hunt_results()
	// plugin.
	if arg.mode == MODE_HUNT_ARTIFACT {
		new_args := ordereddict.NewDict().
			Set("hunt_id", arg.HuntId).
			Set("artifact", arg.Artifact).
			Set("source", arg.Source).
			Set("orgs", arg.OrgIds)

		// Just delegate to the hunt_results() plugin.
		return hunts.HuntResultsPlugin{}.Call(ctx, scope, new_args)
	}

	// Event artifacts just proxy for the monitoring plugin.
	if arg.mode == MODE_EVENT_ARTIFACT {
		new_args := ordereddict.NewDict().
			Set("client_id", arg.ClientId).
			Set("artifact", arg.Artifact).
			Set("source", arg.Source).
			Set("start_row", arg.StartRow)

		if !utils.IsNil(arg.StartTime) {
			new_args.Set("start_time", arg.StartTime)
		}

		if !utils.IsNil(arg.EndTime) {
			new_args.Set("end_time", arg.EndTime)
		}

		// Just delegate directly to the monitoring plugin.
		return MonitoringPlugin{}.Call(ctx, scope, new_args)
	}

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "source", args)()

		// Depending on the mode, we need to read from different
		// places.
		var err error
		var result_set_reader result_sets.ResultSetReader

		if arg.mode == MODE_NOTEBOOK {
			result_set_reader, err = getNotebookResultSetReader(ctx, config_obj, scope, arg)

		} else if arg.mode == MODE_FLOW_ARTIFACT {
			result_set_reader, err = getFlowResultSetReader(ctx, config_obj, scope, arg)

		} else {
			scope.Log("source: unknown arg mode")
			return
		}

		if err != nil {
			scope.Log("source: %v", err)
			return
		}

		if arg.StartRow > 0 {
			err = result_set_reader.SeekToRow(arg.StartRow)
			if errors.Is(err, io.EOF) {
				return
			}

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
		Name:     "source",
		Doc:      "Retrieve rows from stored result sets. This is a one stop show for retrieving stored result set for post processing.",
		ArgType:  type_map.AddType(scope, &SourcePluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.READ_RESULTS).Build(),
	}
}

// Figure out if the artifact is an event artifact based on its
// definition.
func (self *SourcePluginArgs) isArtifactEvent(
	scope vfilter.Scope, ctx context.Context,
	config_obj *config_proto.Config) (is_event bool, client_id string, err error) {

	repository, err := vql_utils.GetRepository(scope)
	if err != nil {
		return false, "", err
	}

	artifact_definition, pres := repository.Get(ctx, config_obj, self.Artifact)
	if !pres {
		return false, "", fmt.Errorf("Artifact %v not known", self.Artifact)
	}

	switch artifact_definition.Type {
	case "client_event":
		if self.ClientId == "" {
			return false, "", fmt.Errorf(
				"Artifact %v is a client event artifact, "+
					"therefore a client id is required.",
				artifact_definition.Name)
		}
		return true, self.ClientId, nil

	case "server_event":
		return true, "server", nil

	default:
		return false, "", nil
	}
}

func getNotebookResultSetReader(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	arg *SourcePluginArgs) (result_sets.ResultSetReader, error) {

	// Version not specified, we need to fetch the current one.
	if arg.NotebookCellVersion == "" {
		notebook_manager, err := services.GetNotebookManager(config_obj)
		if err != nil {
			return nil, err
		}

		cell, err := notebook_manager.GetNotebookCell(ctx, arg.NotebookId,
			arg.NotebookCellId, arg.NotebookCellVersion)
		if err != nil {
			return nil, err
		}

		arg.NotebookCellVersion = cell.CurrentVersion
	}

	table := arg.NotebookCellTable
	if table == 0 {
		table = 1
	}

	file_store_factory := file_store.GetFileStore(config_obj)
	path_manager := paths.NewNotebookPathManager(arg.NotebookId).
		Cell(arg.NotebookCellId, arg.NotebookCellVersion).
		QueryStorage(table)

	return result_sets.NewResultSetReader(
		file_store_factory, path_manager.Path())
}

func getFlowResultSetReader(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	arg *SourcePluginArgs) (result_sets.ResultSetReader, error) {

	file_store_factory := file_store.GetFileStore(config_obj)

	path_manager, err := artifact_paths.NewArtifactPathManager(ctx,
		config_obj, arg.ClientId, arg.FlowId, arg.Artifact)
	if err != nil {
		return nil, err
	}

	return result_sets.NewResultSetReader(
		file_store_factory, path_manager.Path())

}

// Override SourcePluginArgs from the scope.
func (self *SourcePluginArgs) ParseSourceArgsFromScope(scope vfilter.Scope) {
	if self.ClientId == "" {
		client_id, pres := scope.Resolve("ClientId")
		if pres {
			self.ClientId, _ = client_id.(string)
		}
	}

	if self.FlowId == "" {
		flow_id, pres := scope.Resolve("FlowId")
		if pres {
			self.FlowId, _ = flow_id.(string)
		}
	}

	if self.Artifact == "" {
		artifact_name, pres := scope.Resolve("ArtifactName")
		if pres {
			artifact, ok := artifact_name.(string)
			if ok {
				self.Artifact = artifact
			}
		}
	}

	if self.StartTime == "" {
		start_time, pres := scope.Resolve("StartTime")
		if pres {
			self.StartTime = start_time
		}
	}

	if self.EndTime == "" {
		end_time, pres := scope.Resolve("EndTime")
		if pres {
			self.EndTime = end_time
		}
	}

	if self.NotebookId == "" {
		notebook_id, pres := scope.Resolve("NotebookId")
		if pres {
			self.NotebookId, _ = notebook_id.(string)
		}
	}

	if self.NotebookCellId == "" {
		notebook_cell_id, pres := scope.Resolve("NotebookCellId")
		if pres {
			self.NotebookCellId, _ = notebook_cell_id.(string)
		}
	}

	if self.NotebookCellTable == 0 {
		notebook_cell_table, pres := scope.Resolve("NotebookCellTable")
		if pres {
			self.NotebookCellTable, _ = notebook_cell_table.(int64)
		}
	}

	if self.StartRow == 0 {
		start_row, pres := scope.Resolve("StartRow")
		if pres {
			self.StartRow, _ = start_row.(int64)
		}
	}

	if self.Limit == 0 {
		limit, pres := scope.Resolve("Limit")
		if pres {
			self.Limit, _ = limit.(int64)
		}
	}

	if self.HuntId == "" {
		hunt_id, pres := scope.Resolve("HuntId")
		if pres {
			self.HuntId, _ = hunt_id.(string)
		}
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
		defer vql_subsystem.RegisterMonitor(ctx, "flow_results", args)()

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("flow_results: %s", err)
			return
		}

		arg := &FlowResultsPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("flow_results: %v", err)
			return
		}

		err = services.RequireFrontend()
		if err != nil {
			scope.Log("flow_results: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("flow_results: Command can only run on the server")
			return
		}

		// If no artifact is specified, get the first one from
		// the flow.
		if arg.Artifact == "" {
			launcher, err := services.GetLauncher(config_obj)
			if err != nil {
				scope.Log("flow_results: %v", err)
				return
			}
			flow, err := launcher.GetFlowDetails(
				ctx, config_obj, services.GetFlowOptions{},
				arg.ClientId, arg.FlowId)
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

		path_manager, err := artifact_paths.NewArtifactPathManager(ctx,
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
		Name:     "flow_results",
		Doc:      "Retrieve the results of a flow.",
		ArgType:  type_map.AddType(scope, &FlowResultsPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.READ_RESULTS).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&SourcePlugin{})
	vql_subsystem.RegisterPlugin(&FlowResultsPlugin{})
}
