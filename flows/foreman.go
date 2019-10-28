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
//    timestamp of the most recent hunt it ran (as well latest event
//    table version).

// 2. If a newer hunt exists, the foreman sends the hunt_condition
//    query to the client with the response directed to the
//    System.Hunt.Participation artifact monitoring queue.

// 3. The hunt manager service scans the System.Hunt.Participation
//    monitoring queue and launches the relevant flows on each client.

package flows

import (
	"context"

	errors "github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

// ForemanProcessMessage processes a ForemanCheckin message from the
// client.
func ForemanProcessMessage(
	config_obj *config_proto.Config,
	client_id string,
	foreman_checkin *actions_proto.ForemanCheckin) error {

	if foreman_checkin == nil {
		return errors.New("Expected args of type ForemanCheckin")
	}

	// Update the client's event tables.
	if foreman_checkin.LastEventTableVersion < services.GetClientEventsVersion() {
		err := QueueMessageForClient(
			config_obj, client_id,
			services.GetClientUpdateEventTableMessage())
		if err != nil {
			return err
		}
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
		// Hunt is stopped we dont care about it.
		if hunt.State != api_proto.Hunt_RUNNING {
			return nil
		}

		// This hunt is not relevant to this client.
		if hunt.StartTime <= client_last_timestamp {
			return nil
		}

		flow_condition_query, err := calculateFlowConditionQuery(
			config_obj, hunt)
		if err != nil {
			return err
		}

		err = QueueMessageForClient(
			config_obj, client_id,
			&crypto_proto.GrrMessage{
				SessionId:       constants.MONITORING_WELL_KNOWN_FLOW,
				RequestId:       processVQLResponses,
				VQLClientAction: flow_condition_query})
		if err != nil {
			return err
		}

		err = QueueMessageForClient(
			config_obj, client_id,
			&crypto_proto.GrrMessage{
				SessionId: constants.MONITORING_WELL_KNOWN_FLOW,
				RequestId: constants.IgnoreResponseState,
				UpdateForeman: &actions_proto.ForemanCheckin{
					LastHuntTimestamp: hunt.StartTime,
				},
			})
		if err != nil {
			return err
		}

		api_client, cancel := dispatcher.APIClientFactory.GetAPIClient(
			config_obj)
		defer cancel()

		_, err = api_client.NotifyClients(
			context.Background(), &api_proto.NotificationRequest{
				ClientId: client_id})
		return err
	})
}

func calculateFlowConditionQuery(
	config_obj *config_proto.Config,
	hunt *api_proto.Hunt) (
	*actions_proto.VQLCollectorArgs, error) {

	// TODO.

	default_query := getDefaultCollectorArgs(hunt.HuntId)
	err := artifacts.Obfuscate(config_obj, default_query)
	return default_query, err
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
				Name: "System.Hunt.Participation",
				VQL: "SELECT now() as Timestamp, Fqdn, HuntId, " +
					"true as Participate from info()",
			},
		},
	}
}
