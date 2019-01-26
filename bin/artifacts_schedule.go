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

	"github.com/golang/protobuf/ptypes"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/grpc_client"
)

var (
	artifact_command_schedule = artifact_command.Command(
		"schedule", "Schedule an artifact collection for clients.")

	artifact_command_schedule_client = artifact_command_schedule.Arg(
		"client", "The client to schedule collection on. "+
			"This can be a client id or a hostname").
		Required().String()

	artifact_command_schedule_names = artifact_command_schedule.Arg(
		"names", "A list of artifacts to collect.").
		Required().Strings()

	artifact_command_schedule_parameters = artifact_command_schedule.Flag(
		"parameters", "Parameters to set for the artifacts.").
		Short('p').StringMap()
)

func doArtifactsSchedule() {
	request := &flows_proto.ArtifactCollectorArgs{
		Artifacts: &flows_proto.Artifacts{
			Names: *artifact_command_schedule_names,
		}}
	for k, v := range *artifact_command_schedule_parameters {
		request.Parameters.Env = append(request.Parameters.Env,
			&actions_proto.VQLEnv{
				Key: k, Value: v,
			})
	}

	flow_args, _ := ptypes.MarshalAny(request)
	flow_request := &flows_proto.FlowRunnerArgs{
		ClientId: *artifact_command_schedule_client,
		FlowName: "ArtifactCollector",
		Args:     flow_args,
	}

	// Just start an artifact collector flow using the gRPC API.
	config_obj := get_config_or_default()
	channel := grpc_client.GetChannel(config_obj)
	defer channel.Close()

	client := api_proto.NewAPIClient(channel)
	response, err := client.LaunchFlow(context.Background(), flow_request)
	kingpin.FatalIfError(err, "Starting Artifact collector flow ")

	fmt.Printf("Started a new flow (%v)\n", response.FlowId)
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case artifact_command_schedule.FullCommand():
			doArtifactsSchedule()

		default:
			return false
		}
		return true
	})
}
