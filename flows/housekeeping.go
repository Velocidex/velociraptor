/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2025 Rapid7 Inc.

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

package flows

import (
	"context"

	"github.com/Velocidex/ordereddict"
	errors "github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	utils "www.velocidex.com/golang/velociraptor/utils"
)

var (
	clientEventUpdateCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "client_event_update",
		Help: "Total number of client Event Table Update messages sent.",
	})
)

func CheckClientStatus(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string) error {

	client_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return err
	}

	stats, err := client_manager.GetStats(ctx, client_id)
	if err != nil {
		// No client record was found yet. This is ok and can happen
		// if the host is not properly enrolled yet.
		return nil
	}

	// Check the client's event table for validity.
	client_event_manager, err := services.ClientEventManager(config_obj)
	if err != nil {
		return err
	}

	if client_event_manager != nil &&
		client_event_manager.CheckClientEventsVersion(
			ctx, config_obj, client_id, stats.LastEventTableVersion) {

		update_message := client_event_manager.GetClientUpdateEventTableMessage(
			ctx, config_obj, client_id)

		if update_message.UpdateEventTable == nil {
			return errors.New("Invalid event update")
		}

		// Inform the client manager that this client will now receive
		// the latest event table.
		err := client_manager.UpdateStats(ctx, client_id, &services.Stats{
			LastEventTableVersion: update_message.UpdateEventTable.Version,
		})
		if err != nil {
			return err
		}

		clientEventUpdateCounter.Inc()
		err = client_manager.QueueMessageForClient(
			ctx, client_id, update_message,
			services.NOTIFY_CLIENT, utils.BackgroundWriter)
		if err != nil {
			return err
		}
	}

	// Check the client's hunt status
	// Process any needed hunts.
	dispatcher, err := services.GetHuntDispatcher(config_obj)
	if err != nil {
		return err
	}

	// If the client is already up to date we dont need to look
	// further.
	hunts_last_timestamp := dispatcher.GetLastTimestamp()
	if stats.LastHuntTimestamp >= hunts_last_timestamp {
		return nil
	}

	// Take a snapshot of the hunts that we need to run on this
	// client to reduce the time under lock.
	hunts := make([]*api_proto.Hunt, 0)
	err = dispatcher.ApplyFuncOnHunts(ctx, services.OnlyRunningHunts,
		func(hunt *api_proto.Hunt) error {
			// Hunt is stopped we dont care about it.
			if hunt.State != api_proto.Hunt_RUNNING {
				return nil
			}

			// This hunt is not relevant to this client.
			if hunt.StartTime <= stats.LastHuntTimestamp {
				return nil
			}

			// Take a snapshot of the hunt id and start time.
			hunts = append(hunts, &api_proto.Hunt{
				HuntId:    hunt.HuntId,
				StartTime: hunt.StartTime,
			})

			return nil
		})

	if err != nil {
		return err
	}

	// This will now only contains hunts launched since the last hunt
	// timestamp. If it is empty there is nothing to do, return
	if len(hunts) == 0 {
		return nil
	}

	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	// Record the latest timestamp
	latest_timestamp := uint64(0)
	for _, hunt := range hunts {
		// Notify the hunt manager that we need to hunt this client.
		journal.PushRowsToArtifactAsync(ctx, config_obj,
			ordereddict.NewDict().
				Set("HuntId", hunt.HuntId).
				Set("ClientId", client_id),
			"System.Hunt.Participation")

		if hunt.StartTime > latest_timestamp {
			latest_timestamp = hunt.StartTime
		}
	}

	return client_manager.UpdateStats(ctx, client_id, &services.Stats{
		LastHuntTimestamp: latest_timestamp,
	})
}
