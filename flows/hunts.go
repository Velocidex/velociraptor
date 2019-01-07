// Manage in memory hunt replication.  For performance, the hunts
// table is mirrored in memory and refreshed periodically. The clients
// are then compared against it on each poll and hunts are dispatched
// as needed.
package flows

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"path"
	"sort"
	"time"

	"github.com/golang/protobuf/ptypes"
	errors "github.com/pkg/errors"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/services"
	urns "www.velocidex.com/golang/velociraptor/urns"
	"www.velocidex.com/golang/velociraptor/utils"
)

func GetNewHuntId() string {
	result := make([]byte, 8)
	buf := make([]byte, 4)

	rand.Read(buf)
	hex.Encode(result, buf)

	return urns.BuildURN("hunts", constants.HUNT_PREFIX+string(result))
}

func FindCollectedArtifacts(hunt *api_proto.Hunt) {
	if hunt == nil || hunt.StartRequest == nil {
		return
	}

	switch hunt.StartRequest.FlowName {
	case "ArtifactCollector":
		flow_args := &flows_proto.ArtifactCollectorArgs{}
		err := ptypes.UnmarshalAny(hunt.StartRequest.Args, flow_args)
		if err == nil {
			hunt.Artifacts = flow_args.Artifacts.Names
		}
	case "FileFinder":
		hunt.Artifacts = []string{constants.FileFinderArtifactName}
	}
}

func CreateHunt(
	ctx context.Context,
	config_obj *api_proto.Config,
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

	err = db.SetSubject(config_obj, hunt.HuntId, hunt)
	if err != nil {
		return nil, err
	}

	// Trigger a refresh of the hunt dispatcher. This guarantees
	// that fresh data will be read in subsequent ListHunt()
	// calls.
	services.GetHuntDispatcher().Refresh()

	return &hunt.HuntId, nil
}

func ListHunts(config_obj *api_proto.Config, in *api_proto.ListHuntsRequest) (
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
			result.Items = append(result.Items, hunt)
			return nil
		})

	sort.Slice(result.Items, func(i, j int) bool {
		return result.Items[i].CreateTime > result.Items[j].CreateTime
	})

	return result, nil
}

func GetHunt(config_obj *api_proto.Config, in *api_proto.GetHuntRequest) (
	hunt *api_proto.Hunt, err error) {

	var result *api_proto.Hunt

	services.GetHuntDispatcher().ModifyHunt(
		path.Base(in.HuntId),
		func(hunt_obj *api_proto.Hunt) error {
			// Make a copy
			result = &(*hunt_obj)

			// HACK: Velociraptor only knows how to
			// collect artifacts now. Eventually the whole
			// concept of a flow will go away but for now
			// we need to figure out which artifacts we
			// are actually collecting - there are not
			// many possibilities since we have reduced
			// the number of possible flows significantly.
			FindCollectedArtifacts(result)

			return nil
		})

	if result == nil {
		return nil, errors.New("Not found")
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
func ModifyHunt(config_obj *api_proto.Config, hunt_modification *api_proto.Hunt) error {
	utils.Debug(hunt_modification)

	dispatcher := services.GetHuntDispatcher()
	err := dispatcher.ModifyHunt(
		path.Base(hunt_modification.HuntId),
		func(hunt *api_proto.Hunt) error {
			// We are trying to start the hunt.
			if hunt_modification.State == api_proto.Hunt_RUNNING {

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

			err = db.SetSubject(config_obj, hunt.HuntId, hunt)
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
	channel := grpc_client.GetChannel(config_obj)
	defer channel.Close()

	client := api_proto.NewAPIClient(channel)
	client.NotifyClients(
		context.Background(), &api_proto.NotificationRequest{
			NotifyAll: true,
		})

	return nil
}
