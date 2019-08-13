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
	"io"
	"sort"
	"strings"
	"time"

	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

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
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		arg := &SourcePluginArgs{}
		any_config_obj, _ := scope.Resolve("server_config")
		config_obj, ok := any_config_obj.(*config_proto.Config)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		// This plugin will take parameters from environment
		// parameters. This allows its use to be more concise in
		// reports etc where many parameters can be inferred from
		// context.
		parseSourceArgsFromScope(arg, scope)

		// Allow the plugin args to override the environment scope.
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("source: %v", err)
			return
		}

		// Hunt mode is just a proxy for the hunt_results()
		// plugin.
		if arg.Mode == "HUNT" {
			args := vfilter.NewDict().
				Set("hunt_id", arg.HuntId).
				Set("artifact", arg.Artifact).
				Set("source", arg.Source)

			// Just delegate to the hunt_results() plugin.
			plugin := &HuntResultsPlugin{}
			for row := range plugin.Call(ctx, scope, args) {
				output_chan <- row
			}
			return
		}

		// Figure out the mode by looking at the artifact type.
		if arg.Mode == "" {
			repository, _ := artifacts.GetGlobalRepository(config_obj)
			artifact, pres := repository.Get(arg.Artifact)
			if !pres {
				scope.Log("Artifact %s not known", arg.Artifact)
				return
			}
			arg.Mode = artifact.Type
		}

		mode := artifacts.ModeNameToMode(arg.Mode)
		if mode == 0 {
			scope.Log("Invalid mode %v", arg.Mode)
			return
		}

		// Find the glob for the CSV files making up these results.
		csv_path := artifacts.GetCSVPath(
			arg.ClientId, "*",
			arg.FlowId, arg.Artifact, arg.Source, mode)
		if csv_path == "" {
			scope.Log("Invalid mode %v", arg.Mode)
			return
		}

		globber := make(glob.Globber)
		accessor := file_store.GetFileStoreFileSystemAccessor(config_obj)
		globber.Add(csv_path, accessor.PathSplit)

		// Expanding the glob is not sorted but we really need
		// to go in order of dates.
		hits := []string{}
		for hit := range globber.ExpandWithContext(ctx, "", accessor) {
			hits = append(hits, hit.FullPath())
		}
		sort.Strings(hits)

		for _, hit := range hits {
			ts_start, ts_end := parseFileTimestamp(hit)

			// Skip files modified before the required
			// start time.
			if ts_end < arg.StartTime {
				continue
			}

			if arg.EndTime > 0 && ts_start >= arg.EndTime {
				return
			}

			err := self.ScanLog(ctx, config_obj,
				scope, arg, output_chan, hit)
			if err != nil {
				scope.Log("Error reading %v: %v", hit, err)
			}
		}
	}()

	return output_chan
}

func (self SourcePlugin) ScanLog(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope *vfilter.Scope,
	arg *SourcePluginArgs,
	output_chan chan<- vfilter.Row,
	log_path string) error {

	file_store_factory := file_store.GetFileStore(config_obj)
	fd, err := file_store_factory.ReadFile(log_path)
	if err != nil {
		return err
	}
	defer fd.Close()

	csv_reader := csv.NewReader(fd)
	headers, err := csv_reader.Read()
	if err != nil {
		return err
	}

	row_to_dict := func(row_data []interface{}) *vfilter.Dict {
		row := vfilter.NewDict()

		for idx, row_item := range row_data {
			if idx > len(headers) {
				break
			}
			// Event logs have a _ts column representing
			// the time of each event.
			column_name := headers[idx]
			if column_name == "_ts" {
				timestamp, ok := row_item.(int)
				if ok {
					if timestamp < int(arg.StartTime) {
						return nil
					}

					if arg.EndTime > 0 && timestamp > int(arg.EndTime) {
						return nil
					}
				}
			}

			row.Set(column_name, row_item)
		}
		return row
	}

	for {
		select {
		case <-ctx.Done():
			return nil

		default:
			row_data, err := csv_reader.ReadAny()
			if err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}

			dict := row_to_dict(row_data)
			if dict == nil {
				break
			}
			output_chan <- dict
		}
	}
}

func (self SourcePlugin) Info(
	scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "source",
		Doc:     "Retrieve rows from an artifact's source.",
		ArgType: type_map.AddType(scope, &SourcePluginArgs{}),
	}
}

// Derive the unix timestamp from the filename.
func parseFileTimestamp(filename string) (int64, int64) {
	for _, component := range utils.SplitComponents(filename) {
		component = strings.Split(component, ".")[0]
		ts, err := time.Parse("2006-01-02", component)
		if err == nil {
			start := ts.Unix()
			return start, start + 60*60*24
		}
	}
	return 0, 0
}

// Override SourcePluginArgs from the scope.
func parseSourceArgsFromScope(arg *SourcePluginArgs, scope *vfilter.Scope) {
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

func init() {
	vql_subsystem.RegisterPlugin(&SourcePlugin{})
}
