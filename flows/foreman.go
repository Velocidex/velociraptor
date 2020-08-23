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

	"github.com/Velocidex/ordereddict"
	errors "github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

// ForemanProcessMessage processes a ForemanCheckin message from the
// client.
func ForemanProcessMessage(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string,
	foreman_checkin *actions_proto.ForemanCheckin) error {

	if foreman_checkin == nil {
		return errors.New("Expected args of type ForemanCheckin")
	}

	// Update the client's event tables.
	client_event_manager := services.ClientEventManager()
	if client_event_manager.CheckClientEventsVersion(
		config_obj, client_id, foreman_checkin.LastEventTableVersion) {
		err := QueueMessageForClient(
			config_obj, client_id,
			client_event_manager.GetClientUpdateEventTableMessage(
				config_obj, client_id))
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

		// Notify the hunt manager that we need to hunt this client.
		err := services.GetJournal().PushRowsToArtifact(config_obj,
			[]*ordereddict.Dict{ordereddict.NewDict().
				Set("HuntId", hunt.HuntId).
				Set("ClientId", client_id).
				Set("Participate", true),
			}, "System.Hunt.Participation", client_id, "")
		if err != nil {
			return err
		}

		// Let the client know it needs to update its foreman state.
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

		return services.GetNotifier().NotifyListener(config_obj, client_id)
	})
}
