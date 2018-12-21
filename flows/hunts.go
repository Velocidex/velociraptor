// Manage in memory hunt replication.  For performance, the hunts
// table is mirrored in memory and refreshed periodically. The clients
// are then compared against it on each poll and hunts are dispatched
// as needed.
package flows

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes"
	errors "github.com/pkg/errors"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/logging"
	urns "www.velocidex.com/golang/velociraptor/urns"
)

var (
	dispatch_container = &HuntDispatcherContainer{}
)

type HuntDispatcher struct {
	config_obj     *api_proto.Config
	last_timestamp uint64
	hunts          []*api_proto.Hunt
}

func (self *HuntDispatcher) GetApplicableHunts(last_timestamp uint64) []*api_proto.Hunt {
	var result []*api_proto.Hunt

	for _, hunt := range self.hunts {
		if hunt.CreateTime > last_timestamp {
			result = append(result, hunt)
		}
	}
	return result
}

// Check all the hunts in our hunt list for pending clients that
// should have been added.

// Clients are added to hunts in a pre-determined rate (e.g. 20
// clients/min), therefore we need to manage how many clients to be
// added to each hunt. The foreman adds clients to the pending queue
// and the HuntManager takes clients from the pending queue and adds
// them to the running queue at the pre-determined rate.
func (self *HuntDispatcher) Update() error {
	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}
	for _, hunt := range self.hunts {
		// If the hunt is not in the running state we do not
		// schedule new clients for it.
		if hunt.State != api_proto.Hunt_RUNNING {
			continue
		}
		modified, err := self._ScheduleClientsForHunt(hunt)
		if err != nil {
			logger.Error("_ScheduleClientsForHunt:", err)
		}

		// Spin here until all the results are processed for this hunt.
		for {
			modified2, result_count, err := self._SortResultsForHunt(hunt)
			if err != nil {
				logger.Error("_SortResultsForHunt:", err)
			}

			if result_count == 0 {
				break
			}

			if modified2 {
				modified = true
			}
		}

		if modified {
			err = db.SetSubject(self.config_obj, hunt.HuntId, hunt)
			if err != nil {
				logger.Error("", err)
			}
		}

	}
	return nil
}

func (self *HuntDispatcher) _SortResultsForHunt(hunt *api_proto.Hunt) (
	modified bool, result_count int, err error) {
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return false, 0, err
	}

	completed_urn := hunt.HuntId + "/completed"
	// Take the first 100 urns off the list. They will be
	// removed below.
	urns, err := db.ListChildren(
		self.config_obj, completed_urn, 0, 100)
	if err != nil {
		return false, 0, err
	}

	// Nothing to do here.
	if len(urns) == 0 {
		return false, 0, nil
	}

	// Whatever happens we remove these ones.
	defer func() {
		for _, urn := range urns {
			derr := db.DeleteSubject(self.config_obj, urn)
			if derr != nil {
				err = derr
			}
		}
	}()

	for _, urn := range urns {
		summary := &api_proto.HuntInfo{}
		derr := db.GetSubject(self.config_obj, urn, summary)
		var destination string
		if derr != nil || summary.Result == nil ||
			summary.Result.State == flows_proto.FlowContext_ERROR {
			destination = hunt.HuntId + "/errors/" +
				summary.ClientId
			hunt.TotalClientsWithErrors += 1
			err = derr
		} else if summary.Result.TotalResults > 0 {
			destination = hunt.HuntId + "/results/" +
				summary.ClientId
			hunt.TotalClientsWithResults += 1
		} else if summary.Result.TotalResults == 0 {
			destination = hunt.HuntId + "/no_results/" +
				summary.ClientId
			hunt.TotalClientsWithoutResults += 1
		} else {
			continue
		}

		derr = db.SetSubject(self.config_obj, destination, summary)
		if derr != nil {
			err = derr
		}

		modified = true
		result_count += 1
	}

	return
}

// Move the required clients from the pending queue to the running
// queue. We only move clients which are due to be scheduled.
func (self *HuntDispatcher) _ScheduleClientsForHunt(hunt *api_proto.Hunt) (
	modified bool, err error) {
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return false, err
	}

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)

	client_rate := hunt.ClientRate

	// Default client rate is 20 per minute.
	if client_rate == 0 {
		client_rate = 20
	}

	last_unpause_time := hunt.LastUnpauseTime
	// Default LastUnpauseTime is hunt creation time.
	if last_unpause_time == 0 {
		last_unpause_time = hunt.CreateTime
	}
	now := uint64(time.Now().UTC().UnixNano() / 1000)
	seconds_since_unpause := (now - last_unpause_time) / 1000000
	expected_clients := (client_rate*seconds_since_unpause/60 +
		hunt.TotalClientsWhenUnpaused)

	// We should be adding some more clients to the
	// hunt. Read HuntInfo AFF4 objects from the
	// pending queue, launch their flows and put them in
	// the running queue.
	if hunt.TotalClientsScheduled < expected_clients {

		// Only get as many clients as we need from the
		// pending queue and not more.
		clients_to_get := expected_clients - hunt.TotalClientsScheduled
		pending_urn := hunt.HuntId + "/pending"
		urns, err := db.ListChildren(
			self.config_obj, pending_urn, 0, clients_to_get)
		if err != nil {
			return false, err
		}

		// No clients in the pending queue - nothing to do.
		if len(urns) == 0 {
			return false, nil
		}

		// Regardless what happens below we really need to
		// remove the urns from the pending queue.
		defer func() {
			for _, urn := range urns {
				derr := db.DeleteSubject(self.config_obj, urn)
				if derr != nil {
					err = derr
				}
			}
		}()

		// We need to launch the flow by calling our gRPC
		// endpoint API.
		channel := grpc_client.GetChannel(self.config_obj)
		defer channel.Close()

		for _, urn := range urns {
			// Get the summary and launch the flow.
			summary := &api_proto.HuntInfo{}
			err := db.GetSubject(self.config_obj, urn, summary)
			if err != nil {
				logger.Error("", err)
				continue
			}
			flow_runner_args := &flows_proto.FlowRunnerArgs{
				ClientId: summary.ClientId,
				FlowName: "HuntRunnerFlow",
			}
			flow_args, err := ptypes.MarshalAny(summary)
			if err != nil {
				logger.Error("", err)
				continue
			}
			flow_runner_args.Args = flow_args

			client := api_proto.NewAPIClient(channel)
			response, err := client.LaunchFlow(
				context.Background(), flow_runner_args)
			if err != nil {
				// If we can not launch the flow we
				// need to store the summary in the
				// error queue.
				logger.Error("Cant launch hunt flow", err)
				summary.State = api_proto.HuntInfo_ERROR
				summary.Result = &flows_proto.FlowContext{
					CreateTime: uint64(time.Now().UnixNano() / 1000),
					Backtrace:  fmt.Sprintf("HuntDispatcher: %v", err),
				}

				hunt.TotalClientsWithErrors += 1
				modified = true

				error_urn := hunt.HuntId + "/errors/" + summary.ClientId
				err = db.SetSubject(self.config_obj, error_urn, summary)

				continue
			}

			// Store the summary in the running queue.
			summary.FlowId = response.FlowId
			running_urn := hunt.HuntId + "/running/" + summary.ClientId
			err = db.SetSubject(self.config_obj, running_urn, summary)

			hunt.TotalClientsScheduled += 1
			modified = true
		}
	}
	return
}

type HuntDispatcherContainer struct {
	refresh_mu sync.Mutex
	mu         sync.Mutex
	dispatcher *HuntDispatcher
}

func (self *HuntDispatcherContainer) Refresh(config_obj *api_proto.Config) {
	// Serialize access to Refresh() calls. While the
	// NewHuntDispatcher() is being built, readers may access the
	// old one freely, but new Refresh calls are blocked.
	self.refresh_mu.Lock()
	defer self.refresh_mu.Unlock()
	dispatcher, err := NewHuntDispatcher(config_obj)
	if err != nil {
		dispatcher = &HuntDispatcher{}
	}

	// Swap the pointers under lock between the old and new hunt
	// list. This should be very fast minimizing reader
	// contention.
	self.mu.Lock()
	defer self.mu.Unlock()

	self.dispatcher = dispatcher
}

func NewHuntDispatcher(config_obj *api_proto.Config) (*HuntDispatcher, error) {
	result := &HuntDispatcher{config_obj: config_obj}
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	hunts, err := db.ListChildren(config_obj, constants.HUNTS_URN, 0, 100)
	if err != nil {
		return nil, err
	}

	for _, hunt_urn := range hunts {
		hunt_obj := &api_proto.Hunt{}
		err = db.GetSubject(config_obj, hunt_urn, hunt_obj)
		if err != nil {
			return nil, err
		}

		result.hunts = append(result.hunts, hunt_obj)
	}

	err = result.Update()
	if err != nil {
		return nil, err
	}

	return result, nil
}

func GetHuntDispatcher(config_obj *api_proto.Config) (*HuntDispatcher, error) {
	dispatch_container.mu.Lock()
	defer dispatch_container.mu.Unlock()

	if dispatch_container.dispatcher == nil {
		dispatcher, err := NewHuntDispatcher(config_obj)
		if err != nil {
			logging.GetLogger(config_obj, &logging.FrontendComponent).
				Error("", err)
			return nil, err
		}
		dispatch_container.dispatcher = dispatcher

		// Refresh the container every 10 seconds.
		go func() {
			for {
				time.Sleep(10 * time.Second)
				dispatch_container.Refresh(config_obj)
			}
		}()
	}
	return dispatch_container.dispatcher, nil
}

func GetNewHuntId() string {
	result := make([]byte, 8)
	buf := make([]byte, 4)

	rand.Read(buf)
	hex.Encode(result, buf)

	return urns.BuildURN("hunts", constants.HUNT_PREFIX+string(result))
}

func FindCollectedArtifacts(hunt *api_proto.Hunt) {
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

func CreateHunt(config_obj *api_proto.Config, hunt *api_proto.Hunt) (*string, error) {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	hunt.HuntId = GetNewHuntId()
	hunt.CreateTime = uint64(time.Now().UTC().UnixNano() / 1000)
	hunt.LastUnpauseTime = hunt.CreateTime
	if hunt.Expires < hunt.CreateTime {
		hunt.Expires = uint64(time.Now().Add(7*24*time.Hour).
			UTC().UnixNano() / 1000)
	}
	if hunt.State == api_proto.Hunt_UNSET {
		hunt.State = api_proto.Hunt_PAUSED
	}

	err = db.SetSubject(config_obj, hunt.HuntId, hunt)
	if err != nil {
		return nil, err
	}

	// Trigger a refresh of the hunt dispatcher. This
	// guarantees that fresh data will be read in
	// subsequent ListHunt() calls.
	dispatch_container.Refresh(config_obj)

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

	return &hunt.HuntId, nil
}

func ListHunts(config_obj *api_proto.Config, in *api_proto.ListHuntsRequest) (
	*api_proto.ListHuntsResponse, error) {
	dispatcher, err := GetHuntDispatcher(config_obj)
	if err != nil {
		return nil, err
	}

	result := &api_proto.ListHuntsResponse{}
	for idx, hunt := range dispatcher.GetApplicableHunts(0) {
		if uint64(idx) < in.Offset {
			continue
		}

		if uint64(idx) >= in.Offset+in.Count {
			break
		}
		result.Items = append(result.Items, hunt)
	}

	return result, nil
}

func GetHunt(config_obj *api_proto.Config, in *api_proto.GetHuntRequest) (
	*api_proto.Hunt, error) {
	dispatcher, err := GetHuntDispatcher(config_obj)
	if err != nil {
		return nil, err
	}

	for _, hunt := range dispatcher.GetApplicableHunts(0) {
		if path.Base(hunt.HuntId) == in.HuntId {
			// HACK: Velociraptor only knows how to
			// collect artifacts now. Eventually the whole
			// concept of a flow will go away but for now
			// we need to figure out which artifacts we
			// are actually collecting - there are not
			// many possibilities since we have reduced
			// the number of possible flows significantly.
			FindCollectedArtifacts(hunt)
			return hunt, nil
		}
	}

	return nil, errors.New("Not found")
}

func ModifyHunt(config_obj *api_proto.Config, hunt_modification *api_proto.Hunt) error {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	// TODO: Check if the user has permission to start/stop the hunt.
	hunt_obj := &api_proto.Hunt{}
	err = db.GetSubject(config_obj, hunt_modification.HuntId, hunt_obj)
	if err != nil {
		return err
	}
	modified := false

	// Only some modifications are allowed. The modified fields
	// are set in the hunt arg.
	if hunt_modification.State != api_proto.Hunt_UNSET {
		hunt_obj.State = hunt_modification.State
		modified = true

		// Hunt is being unpaused. Adjust the hunt counters to
		// account for the unpause time. If we do not do this,
		// then hunt will schedule all the clients which were
		// not scheduled during the paused interval at once -
		// exceeding the specified client rate.
		if hunt_obj.State == api_proto.Hunt_PAUSED &&
			hunt_modification.State == api_proto.Hunt_RUNNING {
			hunt_obj.LastUnpauseTime = uint64(time.Now().UTC().UnixNano() / 1000)
			hunt_obj.TotalClientsWhenUnpaused = hunt_obj.TotalClientsScheduled
		}
	}

	if modified {
		err := db.SetSubject(config_obj, hunt_modification.HuntId, hunt_obj)
		if err != nil {
			return err
		}

		// Trigger a refresh of the hunt dispatcher. This
		// guarantees that fresh data will be read in
		// subsequent ListHunt() calls.
		dispatch_container.Refresh(config_obj)

		return nil
	}

	return errors.New("Modification not supported.")
}
