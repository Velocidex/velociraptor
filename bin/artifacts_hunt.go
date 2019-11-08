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
package main

import (
	"context"
	"fmt"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/grpc_client"
)

var (
	artifact_command_hunt = artifact_command.Command(
		"hunt", "Hunt for artifacts.")

	artifact_command_hunt_condition = artifact_command_hunt.Flag(
		"condition", "The condition to apply for the hunt.").
		String()

	artifact_command_hunt_names = artifact_command_hunt.Arg(
		"names", "A list of artifacts to collect.").
		Required().Strings()

	artifact_command_hunt_parameters = artifact_command_hunt.Flag(
		"parameters", "Parameters to set for the artifacts.").
		Short('p').StringMap()
)

func doArtifactsHunt() {
	request := &flows_proto.ArtifactCollectorArgs{
		Artifacts: *artifact_command_hunt_names,
	}
	for k, v := range *artifact_command_hunt_parameters {
		request.Parameters.Env = append(request.Parameters.Env,
			&actions_proto.VQLEnv{
				Key: k, Value: v,
			})
	}

	hunt_request := &api_proto.Hunt{
		StartRequest: request,
		State:        api_proto.Hunt_RUNNING,
	}

	if artifact_command_hunt_condition != nil {
		hunt_request.Condition = &api_proto.HuntCondition{}
		hunt_request.Condition.GetGenericCondition().
			FlowConditionQuery = &actions_proto.VQLCollectorArgs{
			Query: []*actions_proto.VQLRequest{
				&actions_proto.VQLRequest{
					VQL: *artifact_command_hunt_condition,
				},
			},
		}
	}

	// Just start an artifact collector hunt using the gRPC API.
	config_obj := get_config_or_default()
	channel := grpc_client.GetChannel(config_obj)
	defer channel.Close()

	client := api_proto.NewAPIClient(channel)
	response, err := client.CreateHunt(context.Background(), hunt_request)
	kingpin.FatalIfError(err, "Starting Artifact collector hunt ")

	fmt.Printf("Created a new hunt (%v)\n", response.FlowId)
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case artifact_command_hunt.FullCommand():
			doArtifactsHunt()

		default:
			return false
		}
		return true
	})
}
