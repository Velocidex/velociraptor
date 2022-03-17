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

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/paths"
	artifact_paths "www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type MonitoringPlugin struct{}

func (self MonitoringPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("monitoring: %s", err)
			return
		}

		arg := &SourcePluginArgs{}

		// Allow the plugin to be filled in from the environment. Arg
		// parser will override the environment with the actual args.
		ParseSourceArgsFromScope(arg, scope)

		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("monitoring: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		// Allow the source to be specified separately but
		// really the full artifact name is required here.
		if arg.Source != "" {
			arg.Artifact = arg.Artifact + "/" + arg.Source
			arg.Source = ""
		}

		path_manager, err := artifact_paths.NewArtifactPathManager(
			config_obj, arg.ClientId, arg.FlowId, arg.Artifact)
		if err != nil {
			scope.Log("monitoring: %v", err)
			return
		}

		file_store_factory := file_store.GetFileStore(config_obj)
		reader, err := result_sets.NewTimedResultSetReader(
			ctx, file_store_factory, path_manager)
		if err != nil {
			scope.Log("monitoring: %v", err)
			return
		}

		if !utils.IsNil(arg.StartTime) {
			start, err := functions.TimeFromAny(scope, arg.StartTime)
			if err == nil {
				err = reader.SeekToTime(start)
				if err != nil {
					scope.Log("monitoring: %v", err)
					return
				}
			}
		}

		if !utils.IsNil(arg.EndTime) {
			end, err := functions.TimeFromAny(scope, arg.EndTime)
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
		Name: "monitoring",
		Doc: "Extract monitoring log from a client. If client_id is not specified " +
			"we watch the global journal which contains event logs from all clients.",
		ArgType: type_map.AddType(scope, &SourcePluginArgs{}),
	}
}

type WatchMonitoringPluginArgs struct {
	Artifact string `vfilter:"optional,field=artifact,doc=The artifact to watch"`
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

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("watch_monitoring: %s", err)
			return
		}

		journal, _ := services.GetJournal()
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

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		mode, err := artifact_paths.GetArtifactMode(
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
		qm_chan, cancel := journal.Watch(ctx, arg.Artifact)

		// Make sure to call this at shutdown (defer is not guaranteed
		// to run).
		scope.AddDestructor(cancel)

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
		ArgType: type_map.AddType(scope, &WatchMonitoringPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&MonitoringPlugin{})
	vql_subsystem.RegisterPlugin(&WatchMonitoringPlugin{})
}
