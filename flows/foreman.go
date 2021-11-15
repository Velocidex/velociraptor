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
	"fmt"

	"github.com/Velocidex/ordereddict"
	errors "github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

var (
	clientEventUpdateCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "client_event_update",
		Help: "Total number of client Event Table Update messages sent.",
	})

	clientHuntTimestampUpdateCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "client_hunt_timestamp_update",
		Help: "Total number of client Update Hunt Timestamp messages sent.",
	})
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
	if client_event_manager != nil &&
		client_event_manager.CheckClientEventsVersion(
			config_obj, client_id,
			foreman_checkin.LastEventTableVersion) {
		clientEventUpdateCounter.Inc()
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
	if dispatcher == nil {
		return nil
	}
	client_last_timestamp := foreman_checkin.LastHuntTimestamp

	// Can we get away without a lock? If the client is already up
	// to date we dont need to look further.
	hunts_last_timestamp := dispatcher.GetLastTimestamp()
	if client_last_timestamp >= hunts_last_timestamp {
		return nil
	}

	// Take a snapshot of the hunts that we need to run on this
	// client to reduce the time under lock.
	hunts := make([]*api_proto.Hunt, 0)
	err := dispatcher.ApplyFuncOnHunts(func(hunt *api_proto.Hunt) error {
		// Hunt is stopped we dont care about it.
		if hunt.State != api_proto.Hunt_RUNNING {
			return nil
		}

		// This hunt is not relevant to this client.
		if hunt.StartTime <= client_last_timestamp {
			return nil
		}

		// Take a snapshot of the hunt id and start time.
		hunts = append(hunts, &api_proto.Hunt{
			HuntId:    hunt.HuntId,
			StartTime: hunt.StartTime,
		})

		return nil
	})

	// Nothing to do, return
	if len(hunts) == 0 {
		return err
	}

	// Now schedule the client for all the hunts that it needs to run.
	journal, err := services.GetJournal()
	if err != nil {
		return err
	}

	// Record the latest timestamp
	latest_timestamp := uint64(0)
	for _, hunt := range hunts {
		fmt.Printf("Notifying %v\n", client_id)
		// Notify the hunt manager that we need to hunt this client.
		journal.PushRowsToArtifactAsync(config_obj,
			ordereddict.NewDict().
				Set("HuntId", hunt.HuntId).
				Set("ClientId", client_id),
			"System.Hunt.Participation")

		if hunt.StartTime > latest_timestamp {
			latest_timestamp = hunt.StartTime
		}
	}

	// Let the client know it needs to update its foreman state to
	// the latest time. We schedule an UpdateForeman message for
	// the client. Note that it is possible that the client does
	// not update its timestamp immediately and therefore might
	// end up sending multiple participation events to the hunt
	// manager - this is ok since the hunt manager keeps hunt
	// participation index and will automatically skip multiple
	// messages.
	clientHuntTimestampUpdateCounter.Inc()
	return QueueMessageForClient(
		config_obj, client_id,
		&crypto_proto.VeloMessage{
			SessionId: constants.MONITORING_WELL_KNOWN_FLOW,
			RequestId: constants.IgnoreResponseState,
			UpdateForeman: &actions_proto.ForemanCheckin{
				LastHuntTimestamp: latest_timestamp,
			},
		})
}
