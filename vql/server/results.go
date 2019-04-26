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

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/flows"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type CollectedArtifactsPluginArgs struct {
	ClientId string `vfilter:"required,field=client_id"`
	FlowId   string `vfilter:"required,field=flow_id"`
	Artifact string `vfilter:"required,field=artifact"`
	Source   string `vfilter:"optional,field=source"`
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

		artifact_name := arg.Artifact
		if arg.Source != "" {
			artifact_name += "/" + arg.Source
		}

		log_path := flows.CalculateArtifactResultPath(
			arg.ClientId, artifact_name, arg.FlowId)

		utils.Debug(log_path)

		file_store_factory := file_store.GetFileStore(config_obj)
		fd, err := file_store_factory.ReadFile(log_path)
		if err != nil {
			scope.Log("Error %v: %v\n", err, log_path)
			return
		}

		// Read each CSV file and emit it with
		// some extra columns for context.
		for row := range csv.GetCSVReader(fd) {
			output_chan <- row.
				Set("ClientId", arg.ClientId).
				Set("FlowId", arg.FlowId)
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
