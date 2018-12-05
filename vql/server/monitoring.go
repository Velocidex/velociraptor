package server

import (
	"context"
	"encoding/csv"
	"path"

	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/file_store"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type MonitoringPluginArgs struct {
	ClientId  string `vfilter:"required,field=client_id"`
	Artifact  string `vfilter:"required,field=artifact"`
	DateRegex string `vfilter:"optional,field=date_regex"`
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
		config_obj, ok := any_config_obj.(*config.Config)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		file_store_factory := file_store.GetFileStore(config_obj)
		log_path := path.Join(
			"clients", arg.ClientId, "monitoring",
			"Artifact "+arg.Artifact)

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
				row := vfilter.NewDict()
				row_data, err := csv_reader.Read()
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

	}()

	return output_chan
}

func (self MonitoringPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "monitoring",
		Doc:     "Extract monitoring log from a client.",
		ArgType: type_map.AddType(scope, &MonitoringPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&MonitoringPlugin{})
}
