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
	"path"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type CollectedArtifactsPluginArgs struct {
	ClientId []string `vfilter:"required,field=client_id"`
	Artifact string   `vfilter:"required,field=artifact"`
}

type CollectedArtifactsPlugin struct{}

func (self CollectedArtifactsPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &CollectedArtifactsPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("collected_artifacts: %v", err)
			return
		}

		any_config_obj, _ := scope.Resolve("server_config")
		config_obj, ok := any_config_obj.(*api_proto.Config)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		for _, client_id := range arg.ClientId {
			log_path := path.Join(
				"clients", client_id, "artifacts",
				"Artifact "+arg.Artifact)

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

				// Read each CSV file and emit it with
				// some extra columns for context.
				for row := range csv.GetCSVReader(fd) {
					output_chan <- row.
						Set("CollectedTime",
							item.ModTime().UnixNano()/1000).
						Set("ClientId", client_id)
				}
			}
		}
	}()

	return output_chan
}

func (self CollectedArtifactsPlugin) Info(
	scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "collected_artifact",
		Doc:     "Retrieve artifacts collected from clients.",
		ArgType: type_map.AddType(scope, &CollectedArtifactsPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&CollectedArtifactsPlugin{})
}
