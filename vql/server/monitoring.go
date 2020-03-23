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
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/glob"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type MonitoringPlugin struct{}

func (self MonitoringPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
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
		err = vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("monitoring: %v", err)
			return
		}

		any_config_obj, _ := scope.Resolve("server_config")
		config_obj, ok := any_config_obj.(*config_proto.Config)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		if arg.DayName == "" {
			arg.DayName = "*"
		}

		// Allow the source to be specified in
		// artifact_name/Source notation.
		artifact_name := arg.Artifact
		source := arg.Source
		if arg.Source == "" {
			artifact_name, source = artifacts.SplitFullSourceName(arg.Artifact)
		}

		// Figure out the mode by looking at the artifact type.
		if arg.Mode == "" {
			repository, _ := artifacts.GetGlobalRepository(config_obj)
			artifact, pres := repository.Get(artifact_name)
			if !pres {
				scope.Log("Artifact %s not known", arg.Artifact)
				return
			}
			arg.Mode = artifact.Type
		}

		mode := artifacts.ModeNameToMode(arg.Mode)
		if mode == 0 {
			scope.Log("Unknown mode %v", arg.Mode)
			return
		}

		log_path := artifacts.GetCSVPath(
			arg.ClientId, arg.DayName,
			arg.FlowId, artifact_name,
			source, mode)

		globber := make(glob.Globber)
		accessor := file_store.GetFileStoreFileSystemAccessor(config_obj)
		globber.Add(log_path, accessor.PathSplit)

		for hit := range globber.ExpandWithContext(ctx, "", accessor) {
			err := self.ScanLog(config_obj,
				scope, output_chan,
				hit.FullPath())
			if err != nil {
				scope.Log(
					"Error reading %v: %v",
					hit.FullPath(), err)
			}
		}
	}()

	return output_chan
}

func (self MonitoringPlugin) ScanLog(
	config_obj *config_proto.Config,
	scope *vfilter.Scope,
	output_chan chan<- vfilter.Row,
	log_path string) error {

	fd, err := file_store.GetFileStore(config_obj).ReadFile(log_path)
	if err != nil {
		return err
	}
	defer fd.Close()

	csv_reader := csv.NewReader(fd)
	headers, err := csv_reader.Read()
	if err != nil {
		return err
	}

	for {
		row := ordereddict.NewDict()
		row_data, err := csv_reader.ReadAny()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		for idx, row_item := range row_data {
			if idx > len(headers) {
				break
			}
			row.Set(headers[idx], row_item)
		}

		output_chan <- row
	}
}

func (self MonitoringPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "monitoring",
		Doc: "Extract monitoring log from a client. If client_id is not specified " +
			"we watch the global journal which contains event logs from all clients.",
		ArgType: type_map.AddType(scope, &MonitoringPluginArgs{}),
	}
}

// Keep the state of each monitoring file.
type state map[string]info

type info struct {
	// Last modification time of the monitoring file.
	age time.Time

	// Last read offset in the file (we tail the file for new items).
	offset int64
}

type MonitoringPluginArgs struct {
	ClientId string `vfilter:"optional,field=client_id,doc=A list of client ids to watch. If not provided we watch all clients."`
	Artifact string `vfilter:"required,field=artifact,doc=The event artifact name to watch"`
	Source   string `vfilter:"optional,field=source,doc=An optional artifact named source"`
}

// The watch_monitoring plugin watches for new rows written to the
// monitoring CSV files on the server.
type WatchMonitoringPlugin struct{}

func (self WatchMonitoringPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		err := vql_subsystem.CheckAccess(scope, acls.COLLECT_SERVER)
		if err != nil {
			scope.Log("watch_monitoring: %s", err)
			return
		}

		arg := &MonitoringPluginArgs{}
		err = vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("watch_monitoring: %v", err)
			return
		}

		any_config_obj, _ := scope.Resolve("server_config")
		config_obj, ok := any_config_obj.(*config_proto.Config)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		// Allow the source to be specified in
		// artifact_name/Source notation.
		artifact_name := arg.Artifact
		source := arg.Source
		if arg.Source == "" {
			artifact_name, source = artifacts.SplitFullSourceName(arg.Artifact)
		}

		repository, _ := artifacts.GetGlobalRepository(config_obj)
		artifact, pres := repository.Get(artifact_name)
		if !pres {
			scope.Log("Artifact %s not known", arg.Artifact)
			return
		}
		mode := artifacts.ModeNameToMode(artifact.Type)
		switch mode {
		case 0:
			scope.Log("Unknown mode %v", artifact.Type)
			return
		case artifacts.MODE_CLIENT, artifacts.MODE_SERVER:
			scope.Log("watch_monitoring only supports monitoring event artifacts")
			return
		}

		globber := make(glob.Globber)
		accessor := file_store.GetFileStoreFileSystemAccessor(config_obj)

		// dir_state contains the initial state of the log
		// files when we first start watching. If the file
		// sizes increase subsequently then we emit new
		// events.
		dir_state := make(state)

		// Otherwise we watch the per client log
		// directory for each client.
		log_path := artifacts.GetCSVPath(
			arg.ClientId, "*", "", artifact_name,
			source, mode)
		globber.Add(log_path, accessor.PathSplit)

		// Capture the initial state of the files. We will
		// only monitor events after this point.
		for item := range globber.ExpandWithContext(ctx, "", accessor) {
			dir_state[item.FullPath()] = info{item.ModTime(), item.Size()}
		}

		// Spin forever here and emit new files or events.
		for {
			for item := range globber.ExpandWithContext(ctx, "", accessor) {
				self.ScanLog(ctx, config_obj,
					scope, dir_state, output_chan,
					item, arg.ClientId, arg.Artifact)
			}

			// Wait and reparse the directory again each second.
			select {

			// Query is cancelled - pack up and go home!
			case <-ctx.Done():
				return

			case <-time.After(time.Second):
			}
		}
	}()

	return output_chan
}

func (self WatchMonitoringPlugin) ScanLog(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope *vfilter.Scope,
	dir_state state,
	output_chan chan<- vfilter.Row,
	item glob.FileInfo,
	client_id string,
	artifact string) {

	file_path := item.FullPath()
	last_info, pres := dir_state[file_path]
	// This is a new file we have not seen before.
	if !pres {
		last_info = info{}
	}

	// Item was not modified since last time, skip it.
	if !item.ModTime().After(last_info.age) {
		return
	}

	// Read the file and parse events from it.
	file_store_factory := file_store.GetFileStore(config_obj)
	fd, err := file_store_factory.ReadFile(file_path)
	if err != nil {
		scope.Log("Error %v: %v\n", err, file_path)
		return
	}
	defer fd.Close()

	// Read the headers.
	csv_reader := csv.NewReader(fd)
	headers, err := csv_reader.Read()
	if err != nil {
		return
	}

	// Seek to the place we left the file last time.
	if last_info.offset > 0 {
		csv_reader.Seek(last_info.offset)
	}

	defer func() {
		// Save the current offset for next time.
		dir_state[file_path] = info{
			item.ModTime(), csv_reader.ByteOffset}
	}()

	for {
		select {
		case <-ctx.Done():
			return

		default:
			row_data, err := csv_reader.ReadAny()
			if err != nil {
				return
			}

			row := ordereddict.NewDict().
				Set("ClientId", client_id).
				Set("Artifact", artifact)

			for idx, row_item := range row_data {
				if idx > len(headers) {
					break
				}
				row.Set(headers[idx], row_item)
			}

			output_chan <- row
		}
	}

}

func (self WatchMonitoringPlugin) Info(scope *vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "watch_monitoring",
		Doc: "Watch clients' monitoring log. This is an event plugin. If " +
			"client_id is not provided we watch the global journal which contains " +
			"events from all clients.",
		ArgType: type_map.AddType(scope, &MonitoringPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&MonitoringPlugin{})
	vql_subsystem.RegisterPlugin(&WatchMonitoringPlugin{})
}
