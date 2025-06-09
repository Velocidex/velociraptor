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

package monitoring

import (
	"context"
	"errors"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	artifact_paths "www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

// Adapted from the flows/results plugin set, this plugin is specifically for
// querying the monitoring_logs from client events.  This is useful for aggregating
// the monitoring_logs into a notebook or other report view to check monitoring health.
// For example, watch logs across clients for ETW session errors or gaps in response times
// that might indicate that a monitor stopped working on a client

type MonitoringLogsPluginArgs struct {
	ClientId string `vfilter:"required,field=client_id,doc=The client id to extract"`

	// Artifacts are specified by name and source. Name may
	// include the source following the artifact name by a slash -
	// e.g. Custom.Artifact/SourceName.
	Artifact string `vfilter:"required,field=artifact,doc=The name of the artifact collection to fetch"`
	Source   string `vfilter:"optional,field=source,doc=An optional named source within the artifact"`

	// If the artifact name specifies an event artifact, you may
	// also specify start and end times to return.
	StartTime vfilter.Any `vfilter:"optional,field=start_time,doc=Start return events from this date (for event sources)"`
	EndTime   vfilter.Any `vfilter:"optional,field=end_time,doc=Stop end events reach this time (event sources)."`
}

type MonitoringLogsPlugin struct{}

func (self MonitoringLogsPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
	if err != nil {
		scope.Log("monitoring_logs: %s", err)
		close(output_chan)
		return output_chan
	}

	arg := &MonitoringLogsPluginArgs{}
	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("monitoring_logs: Command can only run on the server")
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
		scope.Log("monitoring_logs: %v", err)
		close(output_chan)
		return output_chan
	}

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "monitoring_logs", args)()

		// Depending on the parameters, we need to read from
		// different places.
		result_set_reader, err := getResultSetReader(ctx, config_obj, arg)
		if err != nil {
			scope.Log("monitoring_logs: %v", err)
			return
		}

		if !utils.IsNil(arg.StartTime) {
			start_time, err := functions.TimeFromAny(ctx, scope, arg.StartTime)
			if err == nil {
				err = result_set_reader.SeekToTime(start_time)
				if err != nil {
					scope.Log("monitoring_logs: %v", err)
					return
				}
			}
		}

		if !utils.IsNil(arg.EndTime) {
			end, err := functions.TimeFromAny(ctx, scope, arg.EndTime)
			if err == nil {
				result_set_reader.SetMaxTime(end)
			}
		}

		for row := range result_set_reader.Rows(ctx) {
			select {
			case <-ctx.Done():
				return
			case output_chan <- row:
			}
		}
	}()

	return output_chan
}

func (self MonitoringLogsPlugin) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "monitoring_logs",
		Doc:      "Retrieve log messages from client event monitoring for the specified client id and artifact",
		ArgType:  type_map.AddType(scope, &MonitoringLogsPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.READ_RESULTS).Build(),
	}
}

func getResultSetReader(
	ctx context.Context,
	config_obj *config_proto.Config,
	arg *MonitoringLogsPluginArgs) (result_sets.TimedResultSetReader, error) {

	if arg.Artifact != "" {
		if arg.Source != "" {
			arg.Artifact = arg.Artifact + "/" + arg.Source
			arg.Source = ""
		}

		mode := paths.MODE_CLIENT_EVENT
		if arg.ClientId == "server" {
			mode = paths.MODE_SERVER_EVENT
		}

		path_manager := artifact_paths.NewArtifactLogPathManagerWithMode(
			config_obj, arg.ClientId, "", arg.Artifact, mode)
		return result_sets.NewTimedResultSetReader(
			ctx, config_obj, path_manager)
	}

	return nil, errors.New(
		"monitoring_logs: client_id and artifact should be specified.")
}

// Override SourcePluginArgs from the scope.
func ParseSourceArgsFromScope(arg *MonitoringLogsPluginArgs, scope vfilter.Scope) {
	client_id, pres := scope.Resolve("ClientId")
	if pres {
		arg.ClientId, _ = client_id.(string)
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
}

func init() {
	vql_subsystem.RegisterPlugin(&MonitoringLogsPlugin{})
}
