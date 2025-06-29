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
	"www.velocidex.com/golang/velociraptor/paths"
	artifact_paths "www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type MonitoringPluginArgs struct {
	ClientId string `vfilter:"required,field=client_id,doc=The client id to extract"`

	Artifact string `vfilter:"required,field=artifact,doc=The name of the event artifact to read"`
	Source   string `vfilter:"optional,field=source,doc=An optional named source within the artifact"`

	StartTime vfilter.Any `vfilter:"optional,field=start_time,doc=Start return events from this date (for event sources)"`
	EndTime   vfilter.Any `vfilter:"optional,field=end_time,doc=Stop end events reach this time (event sources)."`

	StartRow int64 `vfilter:"optional,field=start_row,doc=Start reading the result set from this row"`
}

type MonitoringPlugin struct{}

func (self MonitoringPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "monitoring", args)()

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("monitoring: %s", err)
			return
		}

		arg := &MonitoringPluginArgs{}

		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("monitoring: %v", err)
			return
		}

		err = services.RequireFrontend()
		if err != nil {
			scope.Log("monitoring: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("monitoring: Command can only run on the server")
			return
		}

		// Allow the source to be specified separately but
		// really the full artifact name is required here.
		if arg.Source != "" {
			arg.Artifact = arg.Artifact + "/" + arg.Source
			arg.Source = ""
		}

		path_manager, err := artifact_paths.NewArtifactPathManager(ctx,
			config_obj, arg.ClientId, "", arg.Artifact)
		if err != nil {
			scope.Log("monitoring: %v", err)
			return
		}

		reader, err := result_sets.NewTimedResultSetReader(
			ctx, config_obj, path_manager)
		if err != nil {
			scope.Log("monitoring: %v", err)
			return
		}

		if !utils.IsNil(arg.StartTime) {
			start, err := functions.TimeFromAny(ctx, scope, arg.StartTime)
			if err == nil {
				err = reader.SeekToTime(start)
				if err != nil {
					scope.Log("monitoring: %v", err)
					return
				}
			}
		}

		if !utils.IsNil(arg.EndTime) {
			end, err := functions.TimeFromAny(ctx, scope, arg.EndTime)
			if err == nil {
				reader.SetMaxTime(end)
			}
		}

		count := int64(0)
		for row := range reader.Rows(ctx) {
			if count < arg.StartRow {
				count++
				continue
			}
			count++

			select {
			case <-ctx.Done():
				return
			case output_chan <- row:
			}
		}
	}()

	return output_chan
}

func (self MonitoringPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "monitoring",
		Doc:      "Read event monitoring log from a client (i.e. that was collected using client event artifacts).",
		ArgType:  type_map.AddType(scope, &MonitoringPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.READ_RESULTS).Build(),
	}
}

type WatchMonitoringPluginArgs struct {
	Artifact string `vfilter:"required,field=artifact,doc=The artifact to watch"`
}

// The watch_monitoring plugin watches for new rows written to the
// monitoring result set files on the server.
type WatchMonitoringPlugin struct{}

func (self WatchMonitoringPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "watch_monitoring", args)()

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("watch_monitoring: %s", err)
			return
		}

		err = services.RequireFrontend()
		if err != nil {
			scope.Log("watch_monitoring: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("watch_monitoring: Command can only run on the server")
			return
		}

		journal, _ := services.GetJournal(config_obj)
		if err != nil {
			return
		}

		if journal == nil {
			scope.Log("watch_monitoring: can only run on the server via the API")
			return
		}

		arg := &WatchMonitoringPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("watch_monitoring: %v", err)
			return
		}

		mode, err := artifact_paths.GetArtifactMode(ctx,
			config_obj, arg.Artifact)
		if err != nil {
			scope.Log("Artifact %s not known", arg.Artifact)
			return
		}

		switch mode {
		case paths.MODE_SERVER_EVENT, paths.MODE_CLIENT_EVENT, paths.INTERNAL:
			break

		default:
			scope.Log("watch_monitoring only supports monitoring event artifacts")
			return
		}

		// Ask the journal service to watch the event queue for us.
		qm_chan, cancel := journal.Watch(
			ctx, arg.Artifact, "watch_monitoring plugin")

		// Make sure to call this at shutdown (defer is not guaranteed
		// to run).
		_ = scope.AddDestructor(cancel)

		for row := range qm_chan {
			select {
			case <-ctx.Done():
				return

			case output_chan <- row:
			}
		}
	}()

	return output_chan
}

func (self WatchMonitoringPlugin) Info(scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "watch_monitoring",
		Doc: "Watch clients' monitoring log. This is an event plugin. If " +
			"client_id is not provided we watch the global journal which contains " +
			"events from all clients.",
		ArgType:  type_map.AddType(scope, &WatchMonitoringPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.READ_RESULTS).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&MonitoringPlugin{})
	vql_subsystem.RegisterPlugin(&WatchMonitoringPlugin{})
}
