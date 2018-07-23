// Manage in memory hunt replication.  For performance, the hunts
// table is mirrored in memory and refreshed periodically. The clients
// are then compared against it on each poll and hunts are dispatched
// as needed.
package flows

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/golang/protobuf/proto"
	"sync"
	"time"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/logging"
	utils "www.velocidex.com/golang/velociraptor/testing"
	urns "www.velocidex.com/golang/velociraptor/urns"
)

var (
	dispatch_container = &HuntDispatcherContainer{}
)

type HuntDispatcher struct {
	config_obj     *config.Config
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
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}
	logger := logging.NewLogger(self.config_obj)
	now := uint64(time.Now().UTC().UnixNano() / 1000)
	for _, hunt := range self.hunts {
		// If the hunt is not in the running state we do not
		// schedule new clients for it.
		if hunt.State != api_proto.Hunt_RUNNING {
			continue
		}

		client_rate := hunt.ClientRate

		// Default client rate is 20 per minute.
		if client_rate == 0 {
			client_rate = 20
		}
		last_unpause_time := hunt.LastUnpauseTime
		if last_unpause_time == 0 {
			last_unpause_time = hunt.CreateTime
		}
		seconds_since_unpause := (now - last_unpause_time) / 1000000
		expected_clients := (client_rate*seconds_since_unpause/60 +
			hunt.TotalClientsWhenUnpaused)
		pending_urn := hunt.HuntId + "/pending"

		// We should be adding some more clients to the hunt.
		if hunt.TotalClientsScheduled < expected_clients {
			clients_to_get := expected_clients - hunt.TotalClientsScheduled
			urns, err := db.ListChildren(
				self.config_obj, pending_urn, 0, clients_to_get)
			if err != nil {
				logger.Error(err.Error())
				continue
			}

			for _, urn := range urns {
				data, err := db.GetSubjectData(self.config_obj, urn, 0, 50)
				if err != nil {
					logger.Error(err.Error())
					continue
				}

				summary := &api_proto.HuntInfo{}
				err = proto.Unmarshal(
					data[constants.HUNTS_SUMMARY_ATTR],
					summary)
				if err != nil {
					logger.Error(err.Error())
					continue
				}

				summary.StartRequest.ClientId = summary.ClientId
				summary.StartRequest.Creator = summary.HuntId
				flow_id, err := StartFlow(
					self.config_obj, summary.StartRequest)
				if err != nil {
					logger.Error(err.Error())
					continue
				}
				summary.FlowId = *flow_id
				running_urn := hunt.HuntId + "/running/" + summary.ClientId
				serialized_summary, err := proto.Marshal(summary)
				if err != nil {
					logger.Error(err.Error())
					continue
				}
				data[constants.HUNTS_SUMMARY_ATTR] = serialized_summary

				err = db.SetSubjectData(
					self.config_obj, running_urn, 0, data)
				if err != nil {
					logger.Error(err.Error())
					continue
				}

				err = db.DeleteSubject(self.config_obj, urn)
				if err != nil {
					logger.Error(err.Error())
					continue
				}

				hunt.TotalClientsScheduled += 1
			}

			serialized_hunt_details, err := proto.Marshal(hunt)
			if err != nil {
				return err
			}

			data := make(map[string][]byte)
			data[constants.HUNTS_INFO_ATTR] = serialized_hunt_details

			err = db.SetSubjectData(self.config_obj, hunt.HuntId, 0, data)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

type HuntDispatcherContainer struct {
	refresh_mu sync.Mutex
	mu         sync.Mutex
	dispatcher *HuntDispatcher
}

func (self *HuntDispatcherContainer) Refresh(config_obj *config.Config) {
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

func NewHuntDispatcher(config_obj *config.Config) (*HuntDispatcher, error) {
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
		hunt_obj, err := GetAFF4HuntObject(config_obj, hunt_urn)
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

func GetHuntDispatcher(config_obj *config.Config) (*HuntDispatcher, error) {
	dispatch_container.mu.Lock()
	defer dispatch_container.mu.Unlock()
	if dispatch_container.dispatcher == nil {
		dispatcher, err := NewHuntDispatcher(config_obj)
		if err != nil {
			logging.NewLogger(config_obj).Error(err.Error())
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

func CreateHunt(config_obj *config.Config, hunt *api_proto.Hunt) (*string, error) {
	utils.Debug(hunt)
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

	err := SetAFF4HuntObject(config_obj, hunt)
	if err != nil {
		return nil, err
	}

	return &hunt.HuntId, nil
}

func ListHunts(config_obj *config.Config, in *api_proto.ListHuntsRequest) (
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

func GetHunt(config_obj *config.Config, in *api_proto.GetHuntRequest) (
	*api_proto.Hunt, error) {
	dispatcher, err := GetHuntDispatcher(config_obj)
	if err != nil {
		return nil, err
	}

	for _, hunt := range dispatcher.GetApplicableHunts(0) {
		if hunt.HuntId == in.HuntId {
			return hunt, nil
		}
	}

	return nil, errors.New("Not found")
}

func ModifyHunt(config_obj *config.Config, hunt *api_proto.Hunt) error {
	// TODO: Check if the user has permission to start/stop the hunt.
	hunt_obj, err := GetAFF4HuntObject(config_obj, hunt.HuntId)
	if err != nil {
		return err
	}

	modified := false

	// Only some modifications are allowed. The modified fields
	// are set int he hunt arg.
	if hunt.State != api_proto.Hunt_UNSET {
		hunt_obj.State = hunt.State
		modified = true

		// Hunt is being unpaused. Adjust the hunt counters to
		// account for the unpause time. If we do not do this,
		// then hunt will schedule all the clients which were
		// not scheduled during the paused interval at once -
		// exceeding the specified client rate.
		if hunt_obj.State == api_proto.Hunt_PAUSED &&
			hunt.State == api_proto.Hunt_RUNNING {
			hunt_obj.LastUnpauseTime = uint64(time.Now().UTC().UnixNano() / 1000)
			hunt_obj.TotalClientsWhenUnpaused = hunt_obj.TotalClientsScheduled
		}
	}

	if modified {
		err := SetAFF4HuntObject(config_obj, hunt_obj)
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

func GetAFF4HuntObject(config_obj *config.Config, hunt_urn string) (*api_proto.Hunt, error) {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	data, err := db.GetSubjectAttributes(
		config_obj, hunt_urn, constants.ATTR_HUNT_OBJECT)
	if err != nil {
		return nil, err
	}

	hunt_obj := &api_proto.Hunt{}
	serialized_hunt, pres := data[constants.HUNTS_INFO_ATTR]
	if pres {
		err := proto.Unmarshal(serialized_hunt, hunt_obj)
		if err != nil {
			return nil, err
		}
	}
	return hunt_obj, nil
}

func SetAFF4HuntObject(config_obj *config.Config, hunt *api_proto.Hunt) error {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	data := make(map[string][]byte)

	serialized_hunt_details, err := proto.Marshal(hunt)
	if err != nil {
		return err
	}

	data[constants.HUNTS_INFO_ATTR] = serialized_hunt_details

	return db.SetSubjectData(config_obj, hunt.HuntId, 0, data)
}
