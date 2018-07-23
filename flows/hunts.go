// Manage in memory hunt replication.  For performance, the hunts
// table is mirrored in memory and refreshed periodically. The clients
// are then compared against it on each poll and hunts are dispatched
// as needed.
package flows

import (
	"crypto/rand"
	"encoding/hex"
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
		client_rate := hunt.ClientRate

		// Default client rate is 20 per minute.
		if client_rate == 0 {
			client_rate = 20
		}
		seconds_since_creation := (now - hunt.CreateTime) / 1000000
		expected_clients := client_rate * seconds_since_creation / 60
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

				utils.Debug(summary)

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
	mu         sync.Mutex
	dispatcher *HuntDispatcher
}

func (self *HuntDispatcherContainer) Refresh(config_obj *config.Config) {
	dispatcher, err := NewHuntDispatcher(config_obj)
	if err != nil {
		dispatcher = &HuntDispatcher{}
	}

	// Swap the pointers under lock between the old and new hunt
	// list. This should be very fast.
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
		data, err := db.GetSubjectAttributes(
			config_obj, hunt_urn, constants.ATTR_HUNT_OBJECT)
		if err != nil {
			return nil, err
		}
		serialized_hunt, pres := data[constants.HUNTS_INFO_ATTR]
		if pres {
			hunt_obj := &api_proto.Hunt{}
			err := proto.Unmarshal(serialized_hunt, hunt_obj)
			if err != nil {
				continue
			}

			result.hunts = append(result.hunts, hunt_obj)
		}
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
	hunt.State = api_proto.Hunt_PENDING

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	data := make(map[string][]byte)

	serialized_hunt_details, err := proto.Marshal(hunt)
	if err != nil {
		return nil, err
	}

	data[constants.HUNTS_INFO_ATTR] = serialized_hunt_details

	err = db.SetSubjectData(config_obj, hunt.HuntId, 0, data)
	if err != nil {
		return nil, err
	}

	return &hunt.HuntId, nil
}
