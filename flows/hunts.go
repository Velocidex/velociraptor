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
// Manage in memory hunt replication.  For performance, the hunts
// table is mirrored in memory and refreshed periodically. The clients
// are then compared against it on each poll and hunts are dispatched
// as needed.
package flows

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"path"
	"sort"
	"time"

	"github.com/golang/protobuf/proto"
	errors "github.com/pkg/errors"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/vfilter"
)

func GetNewHuntId() string {
	result := make([]byte, 8)
	buf := make([]byte, 4)

	rand.Read(buf)
	hex.Encode(result, buf)

	return constants.HUNT_PREFIX + string(result)
}

func FindCollectedArtifacts(
	config_obj *config_proto.Config,
	hunt *api_proto.Hunt) {
	if hunt == nil || hunt.StartRequest == nil ||
		hunt.StartRequest.Artifacts == nil {
		return
	}

	hunt.Artifacts = hunt.StartRequest.Artifacts.Names
	hunt.ArtifactSources = []string{}
	for _, artifact := range hunt.StartRequest.Artifacts.Names {
		for _, source := range artifacts.GetArtifactSources(
			config_obj, artifact) {
			hunt.ArtifactSources = append(
				hunt.ArtifactSources,
				path.Join(artifact, source))
		}
	}
}

func CreateHunt(
	ctx context.Context,
	config_obj *config_proto.Config,
	hunt *api_proto.Hunt) (*string, error) {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	if hunt.Stats == nil {
		hunt.Stats = &api_proto.HuntStats{}
	}

	if hunt.HuntId == "" {
		hunt.HuntId = GetNewHuntId()
	}
	hunt.CreateTime = uint64(time.Now().UTC().UnixNano() / 1000)
	if hunt.Expires < hunt.CreateTime {
		hunt.Expires = uint64(time.Now().Add(7*24*time.Hour).
			UTC().UnixNano() / 1000)
	}

	// We allow our caller to determine if hunts are created in
	// the running state or the paused state.
	if hunt.State == api_proto.Hunt_UNSET {
		hunt.State = api_proto.Hunt_PAUSED

		// IF we are creating the hunt in the running state
		// set it started.
	} else if hunt.State == api_proto.Hunt_RUNNING {
		hunt.StartTime = hunt.CreateTime

		// Notify all the clients about the new hunt. New
		// hunts are not that common so notifying all the
		// clients at once is probably ok.
		channel := grpc_client.GetChannel(config_obj)
		defer channel.Close()

		client := api_proto.NewAPIClient(channel)
		client.NotifyClients(
			context.Background(), &api_proto.NotificationRequest{
				NotifyAll: true,
			})
	}

	err = db.SetSubject(config_obj, constants.GetHuntURN(hunt.HuntId), hunt)
	if err != nil {
		return nil, err
	}

	// Trigger a refresh of the hunt dispatcher. This guarantees
	// that fresh data will be read in subsequent ListHunt()
	// calls.
	services.GetHuntDispatcher().Refresh()

	return &hunt.HuntId, nil
}

func ListHunts(config_obj *config_proto.Config, in *api_proto.ListHuntsRequest) (
	*api_proto.ListHuntsResponse, error) {

	result := &api_proto.ListHuntsResponse{}
	services.GetHuntDispatcher().ApplyFuncOnHunts(
		func(hunt *api_proto.Hunt) error {
			if uint64(len(result.Items)) < in.Offset {
				return nil
			}

			if uint64(len(result.Items)) >= in.Offset+in.Count {
				return errors.New("Stop Iteration")
			}

			if in.IncludeArchived || hunt.State != api_proto.Hunt_ARCHIVED {

				// FIXME: Backwards compatibility.
				hunt.HuntId = path.Base(hunt.HuntId)

				result.Items = append(result.Items, hunt)
			}
			return nil
		})

	sort.Slice(result.Items, func(i, j int) bool {
		return result.Items[i].CreateTime > result.Items[j].CreateTime
	})

	return result, nil
}

func GetHunt(config_obj *config_proto.Config, in *api_proto.GetHuntRequest) (
	hunt *api_proto.Hunt, err error) {

	var result *api_proto.Hunt

	services.GetHuntDispatcher().ModifyHunt(
		in.HuntId,
		func(hunt_obj *api_proto.Hunt) error {
			// HACK: Velociraptor only knows how to
			// collect artifacts now. Eventually the whole
			// concept of a flow will go away but for now
			// we need to figure out which artifacts we
			// are actually collecting - there are not
			// many possibilities since we have reduced
			// the number of possible flows significantly.
			FindCollectedArtifacts(config_obj, hunt_obj)

			// Make a copy
			result = proto.Clone(hunt_obj).(*api_proto.Hunt)

			return nil
		})

	if result == nil {
		return result, errors.New("Not found")
	}

	return result, nil
}

// This method modifies the hunt. Only the following modifications are allowed:

// 1. A hunt in the paused state can go to the running state. This
//    will update the StartTime.
// 2. A hunt in the running state can go to the Stop state

// It is not possible to restart a stopped hunt. This is because the
// hunt manager watches the hunt participation events for all hunts at
// the same time, and just ignores clients that want to participate in
// stopped hunts. It is not possible to go back and re-examine the
// queue.
func ModifyHunt(config_obj *config_proto.Config,
	hunt_modification *api_proto.Hunt,
	user string) error {
	dispatcher := services.GetHuntDispatcher()
	err := dispatcher.ModifyHunt(
		hunt_modification.HuntId,
		func(hunt *api_proto.Hunt) error {
			// Archive the hunt.
			if hunt_modification.State == api_proto.Hunt_ARCHIVED {
				hunt.State = api_proto.Hunt_ARCHIVED

				row := vfilter.NewDict().
					Set("Timestamp", time.Now().UTC().Unix()).
					Set("Hunt", hunt).
					Set("User", user)
				serialized, err := json.Marshal([]vfilter.Row{row})
				if err == nil {
					gJournalWriter.Channel <- &Event{
						Config:    config_obj,
						QueryName: "System.Hunt.Archive",
						Response:  string(serialized),
						Columns:   []string{"Timestamp", "Hunt"},
					}
				}
				// We are trying to start the hunt.
			} else if hunt_modification.State == api_proto.Hunt_RUNNING {

				// The hunt has been expired.
				if hunt.Stats.Stopped {
					return errors.New("Can not start a stopped hunt.")
				}

				hunt.State = api_proto.Hunt_RUNNING
				hunt.StartTime = uint64(time.Now().UnixNano() / 1000)

				// We are trying to pause or stop the hunt.
			} else {
				hunt.State = api_proto.Hunt_STOPPED
			}

			// Write the new hunt object to the datastore.
			db, err := datastore.GetDB(config_obj)
			if err != nil {
				return err
			}

			err = db.SetSubject(
				config_obj,
				constants.GetHuntURN(hunt.HuntId), hunt)
			if err != nil {
				return err
			}

			return nil
		})

	if err != nil {
		return err
	}

	// Notify all the clients about the new hunt. New hunts are
	// not that common so notifying all the clients at once is
	// probably ok.
	client, cancel := dispatcher.APIClientFactory.GetAPIClient(config_obj)
	defer cancel()
	client.NotifyClients(
		context.Background(), &api_proto.NotificationRequest{
			NotifyAll: true,
		})

	return nil
}
