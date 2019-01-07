package server

import (
	"context"
	"path"
	"time"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type MonitoringPluginArgs struct {
	ClientId []string `vfilter:"optional,field=client_id"`
	Artifact string   `vfilter:"required,field=artifact"`
}

type MonitoringPlugin struct{}

func (self MonitoringPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &MonitoringPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("monitoring: %v", err)
			return
		}

		any_config_obj, _ := scope.Resolve("server_config")
		config_obj, ok := any_config_obj.(*api_proto.Config)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		// If no client id is specified, we list the journal
		// which collects events from all clients at once.
		if len(arg.ClientId) == 0 {
			log_path := path.Join(
				"journals",
				"Artifact "+arg.Artifact)

			self.ScanLog(config_obj, scope, output_chan,
				log_path, "", arg.Artifact)
			return
		}

		for _, client_id := range arg.ClientId {
			log_path := path.Join(
				"clients", client_id, "monitoring",
				"Artifact "+arg.Artifact)

			self.ScanLog(config_obj, scope, output_chan,
				log_path, client_id, arg.Artifact)
		}
	}()

	return output_chan
}

func (self MonitoringPlugin) ScanLog(
	config_obj *api_proto.Config,
	scope *vfilter.Scope,
	output_chan chan<- vfilter.Row,
	log_path string,
	client_id string,
	artifact string) {

	file_store_factory := file_store.GetFileStore(config_obj)

	listing, err := file_store_factory.ListDirectory(log_path)
	if err != nil {
		return
	}

	for _, item := range listing {
		file_path := path.Join(log_path, item.Name())
		fd, err := file_store_factory.ReadFile(file_path)
		if err != nil {
			scope.Log("Error %v: %v\n", err, file_path)
			continue
		}

		csv_reader := csv.NewReader(fd)
		headers, err := csv_reader.Read()
		if err != nil {
			continue
		}

	process_file:
		for {
			row := vfilter.NewDict().
				Set("ClientId", client_id).
				Set("Artifact", artifact)

			row_data, err := csv_reader.ReadAny()
			if err != nil {
				break process_file
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

type WatchMonitoringPlugin struct{}

func (self WatchMonitoringPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &MonitoringPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("watch_monitoring: %v", err)
			return
		}

		any_config_obj, _ := scope.Resolve("server_config")
		config_obj, ok := any_config_obj.(*api_proto.Config)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		file_store_factory := file_store.GetFileStore(config_obj)

		// dir_state contains the initial state of the log
		// files when we first start watching. If the file
		// sizes increase subsequently then we emit new
		// events.
		dir_state := make(state)

		// The list of paths to watch.
		watched_paths := []string{}

		// If no client id is specified, we watch the journal
		// which combine events from all clients at the same
		// time.
		if len(arg.ClientId) == 0 {
			watched_paths = append(watched_paths, path.Join(
				"journals", "Artifact "+arg.Artifact))

		} else {

			// Otherwise we watch the per client log
			// directory for each client.
			for _, client_id := range arg.ClientId {
				watched_paths = append(watched_paths, path.Join(
					"clients", client_id, "monitoring",
					"Artifact "+arg.Artifact))
			}
		}

		// Capture the initial state of the files. We will
		// only monitor events after this point.
		for _, log_path := range watched_paths {
			listing, err := file_store_factory.ListDirectory(log_path)
			if err != nil {
				return
			}

			for _, item := range listing {
				file_path := path.Join(log_path, item.Name())
				dir_state[file_path] = info{item.ModTime(), item.Size()}
			}
		}

		// Spin forever here and emit new files or events.
		for {
			// Just scan the journal once.
			if len(arg.ClientId) == 0 {
				log_path := path.Join(
					"journals",
					"Artifact "+arg.Artifact)
				self.ScanLog(ctx, config_obj, scope,
					dir_state, output_chan,
					log_path, "", arg.Artifact)

			} else {
				// Scan all clients and their watched path.
				for idx, client_id := range arg.ClientId {
					log_path := watched_paths[idx]
					self.ScanLog(ctx, config_obj,
						scope, dir_state, output_chan,
						log_path, client_id, arg.Artifact)
				}
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
	config_obj *api_proto.Config,
	scope *vfilter.Scope,
	dir_state state,
	output_chan chan<- vfilter.Row,
	log_path string,
	client_id string,
	artifact string) {

	file_store_factory := file_store.GetFileStore(config_obj)
	listing, err := file_store_factory.ListDirectory(log_path)
	if err != nil {
		return
	}

	for _, item := range listing {
		file_path := path.Join(log_path, item.Name())
		last_info, pres := dir_state[file_path]
		// This is a new file we have not seen before.
		if !pres {
			last_info = info{}
		}

		// Item was not modified since last time, skip it.
		if !item.ModTime().After(last_info.age) {
			continue
		}

		// Read the file and parse events from it.
		fd, err := file_store_factory.ReadFile(file_path)
		if err != nil {
			scope.Log("Error %v: %v\n", err, file_path)
			continue
		}

		// Read the headers.
		csv_reader := csv.NewReader(fd)
		headers, err := csv_reader.Read()
		if err != nil {
			continue
		}

		// Seek to the place we left the file last time.
		if last_info.offset > 0 {
			csv_reader.Seek(last_info.offset)
		}

	process_file:
		for {
			select {
			case <-ctx.Done():
				return

			default:
				row := vfilter.NewDict().
					Set("ClientId", client_id).
					Set("Artifact", artifact)

				row_data, err := csv_reader.ReadAny()
				if err != nil {
					break process_file
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

		// Save the current offset for next time.
		dir_state[file_path] = info{
			item.ModTime(), csv_reader.ByteOffset}
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
