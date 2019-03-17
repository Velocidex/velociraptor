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
// The foreman is a Well Known Flow for clients to check for hunt
// memberships. Each client periodically informs the foreman of the
// most recent hunt it executed, and the foreman launches the relevant
// flow on the client.

// The process goes like this:

// 1. The client sends a message to the foreman periodically with the
//    timestamp of the most recent hunt it ran.

// 2. If a newer hunt exists, the foreman sends the hunt_condition
//    query to the client with the response directed to the
//    System.Hunt.Participation artifact monitoring queue.

// 3. The hunt manager scans the System.Hunt.Participation monitoring
//    queue and launches the relevant flows on each client.

package flows

import (
	"context"

	"github.com/golang/protobuf/ptypes"
	errors "github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	constants "www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/services"
	urns "www.velocidex.com/golang/velociraptor/urns"
)

type Foreman struct {
	BaseFlow
}

func (self *Foreman) New() Flow {
	return &Foreman{BaseFlow{}}
}

func (self *Foreman) ProcessEventTables(
	config_obj *api_proto.Config,
	flow_obj *AFF4FlowObject,
	source string,
	arg *actions_proto.ForemanCheckin) error {

	// Need to update client's event table.
	if arg.LastEventTableVersion < config_obj.Events.Version {
		repository, err := artifacts.GetGlobalRepository(config_obj)
		if err != nil {
			return err
		}

		event_table := &actions_proto.VQLEventTable{
			Version: config_obj.Events.Version,
		}
		for _, name := range config_obj.Events.Artifacts {
			rate := config_obj.Events.OpsPerSecond
			if rate == 0 {
				rate = 100
			}

			vql_collector_args := &actions_proto.VQLCollectorArgs{
				MaxWait:      100,
				OpsPerSecond: rate,
			}
			artifact, pres := repository.Get(name)
			if !pres {
				return errors.New("Unknown artifact " + name)
			}

			err := repository.Compile(artifact, vql_collector_args)
			if err != nil {
				return err
			}
			// Add any artifact dependencies.
			repository.PopulateArtifactsVQLCollectorArgs(vql_collector_args)
			event_table.Event = append(event_table.Event, vql_collector_args)
		}

		channel := grpc_client.GetChannel(config_obj)
		defer channel.Close()

		flow_runner_args := &flows_proto.FlowRunnerArgs{
			ClientId: source,
			FlowName: "MonitoringFlow",
		}
		flow_args, err := ptypes.MarshalAny(event_table)
		if err != nil {
			return errors.WithStack(err)
		}
		flow_runner_args.Args = flow_args
		client := api_proto.NewAPIClient(channel)
		_, err = client.LaunchFlow(context.Background(), flow_runner_args)
		if err != nil {
			return err
		}
	}

	return nil
}

func (self *Foreman) ProcessMessage(
	config_obj *api_proto.Config,
	flow_obj *AFF4FlowObject,
	message *crypto_proto.GrrMessage) error {
	foreman_checkin, ok := responder.ExtractGrrMessagePayload(
		message).(*actions_proto.ForemanCheckin)
	if !ok {
		return errors.New("Expected args of type ForemanCheckin")
	}

	// Update the client's event tables.
	err := self.ProcessEventTables(config_obj, flow_obj, message.Source,
		foreman_checkin)
	if err != nil {
		return err
	}

	// Process any needed hunts.
	dispatcher := services.GetHuntDispatcher()
	client_last_timestamp := foreman_checkin.LastHuntTimestamp

	// Can we get away without a lock?
	hunts_last_timestamp := dispatcher.GetLastTimestamp()
	if client_last_timestamp >= hunts_last_timestamp {
		return nil
	}

	// Nop - we need to lock and examine the hunts more carefully.
	return dispatcher.ApplyFuncOnHunts(func(hunt *api_proto.Hunt) error {
		// This hunt is not relevant to this client.
		if hunt.StartTime <= client_last_timestamp {
			return nil
		}

		flow_condition_query, err := calculateFlowConditionQuery(hunt)
		if err != nil {
			return err
		}

		urn := urns.BuildURN(
			"clients", message.Source,
			"flows", constants.MONITORING_WELL_KNOWN_FLOW)

		err = QueueAndNotifyClient(
			config_obj, message.Source, urn,
			"VQLClientAction",
			flow_condition_query,
			processVQLResponses)
		if err != nil {
			return err
		}

		err = QueueAndNotifyClient(
			config_obj, message.Source, urn,
			"UpdateForeman",
			&actions_proto.ForemanCheckin{
				LastHuntTimestamp: hunt.StartTime,
			}, constants.IgnoreResponseState)
		if err != nil {
			return err
		}
		return nil
	})
}

func calculateFlowConditionQuery(hunt *api_proto.Hunt) (
	*actions_proto.VQLCollectorArgs, error) {

	// TODO.

	return getDefaultCollectorArgs(hunt.HuntId), nil
}

func getDefaultCollectorArgs(hunt_id string) *actions_proto.VQLCollectorArgs {
	return &actions_proto.VQLCollectorArgs{
		Env: []*actions_proto.VQLEnv{
			&actions_proto.VQLEnv{
				Key:   "HuntId",
				Value: hunt_id,
			},
		},
		Query: []*actions_proto.VQLRequest{
			&actions_proto.VQLRequest{
				Name: "Artifact System.Hunt.Participation",
				VQL: "SELECT now() as Timestamp, Fqdn, HuntId, " +
					"true as Participate from info()",
			},
		},
	}
}
