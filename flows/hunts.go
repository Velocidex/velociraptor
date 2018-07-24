// Manage in memory hunt replication.  For performance, the hunts
// table is mirrored in memory and refreshed periodically. The clients
// are then compared against it on each poll and hunts are dispatched
// as needed.
package flows

import (
	"crypto/rand"
	"encoding/hex"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	errors "github.com/pkg/errors"
	"sync"
	"time"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
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
		// Default LastUnpauseTime is hunt creation time.
		if last_unpause_time == 0 {
			last_unpause_time = hunt.CreateTime
		}
		seconds_since_unpause := (now - last_unpause_time) / 1000000
		expected_clients := (client_rate*seconds_since_unpause/60 +
			hunt.TotalClientsWhenUnpaused)

		// We should be adding some more clients to the
		// hunt. Read HuntInfo AFF4 objects from the
		// pending queue, launch their flows and put them in
		// the running queue.
		if hunt.TotalClientsScheduled < expected_clients {
			clients_to_get := expected_clients - hunt.TotalClientsScheduled
			pending_urn := hunt.HuntId + "/pending"
			urns, err := db.ListChildren(
				self.config_obj, pending_urn, 0, clients_to_get)
			if err != nil {
				logger.Error("", err)
				continue
			}

			for _, urn := range urns {
				data, err := db.GetSubjectAttributes(
					self.config_obj, urn,
					constants.ATTR_HUNT_SUMMARY_OBJECT)
				if err != nil {
					logger.Error("", err)
					continue
				}

				summary := &api_proto.HuntInfo{}
				err = proto.Unmarshal(
					data[constants.HUNTS_SUMMARY_ATTR],
					summary)
				if err != nil {
					logger.Error("", err)
					continue
				}

				runner_args := &flows_proto.FlowRunnerArgs{
					ClientId: summary.ClientId,
					FlowName: "HuntRunnerFlow",
				}
				err = SetFlowArgs(runner_args, summary)
				if err != nil {
					logger.Error("", err)
					continue
				}

				flow_id, err := StartFlow(self.config_obj, runner_args)
				if err != nil {
					logger.Error("", err)
					continue
				}
				summary.FlowId = *flow_id
				running_urn := hunt.HuntId + "/running/" + summary.ClientId
				serialized_summary, err := proto.Marshal(summary)
				if err != nil {
					logger.Error("", err)
					continue
				}
				data[constants.HUNTS_SUMMARY_ATTR] = serialized_summary

				err = db.SetSubjectData(
					self.config_obj, running_urn, 0, data)
				if err != nil {
					logger.Error("", err)
					continue
				}

				err = db.DeleteSubject(self.config_obj, urn)
				if err != nil {
					logger.Error("", err)
					continue
				}

				hunt.TotalClientsScheduled += 1
			}

			serialized_hunt_details, err := proto.Marshal(hunt)
			if err != nil {
				return errors.WithStack(err)
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
			logging.NewLogger(config_obj).Error("", err)
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
			return nil, errors.WithStack(err)
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
		return errors.WithStack(err)
	}

	data[constants.HUNTS_INFO_ATTR] = serialized_hunt_details

	return db.SetSubjectData(config_obj, hunt.HuntId, 0, data)
}

// A Flow which runs a delegate flow and stores the result in the
// hunt.
type HuntRunnerFlow struct {
	delegate_flow_obj *AFF4FlowObject
}

func (self *HuntRunnerFlow) Start(
	config_obj *config.Config,
	flow_obj *AFF4FlowObject,
	args proto.Message) error {
	hunt_summary_args, ok := args.(*api_proto.HuntInfo)
	if !ok {
		return errors.New("Expected args of type HuntInfo")
	}
	utils.Debug(hunt_summary_args)
	delegate_flow_obj_proto := &flows_proto.AFF4FlowObject{
		Urn:        flow_obj.Urn,
		RunnerArgs: hunt_summary_args.StartRequest,
	}
	delegate_flow_obj_proto.RunnerArgs.ClientId = hunt_summary_args.ClientId
	delegate_flow_obj_proto.RunnerArgs.Creator = hunt_summary_args.HuntId

	flow_obj.SetState(delegate_flow_obj_proto)

	delegate_args, err := GetFlowArgs(hunt_summary_args.StartRequest)
	if err != nil {
		return err
	}

	delegate_flow_obj, err := AFF4FlowObjectFromProto(delegate_flow_obj_proto)
	if err != nil {
		return err
	}
	err = delegate_flow_obj.impl.Start(
		config_obj, delegate_flow_obj, delegate_args)

	return err
}

func (self *HuntRunnerFlow) Load(
	config_obj *config.Config,
	flow_obj *AFF4FlowObject) error {
	delegate_flow_obj_proto, ok := flow_obj.GetState().(*flows_proto.AFF4FlowObject)
	if ok {
		delegate_flow_obj, err := AFF4FlowObjectFromProto(delegate_flow_obj_proto)
		if err != nil {
			return err
		}
		self.delegate_flow_obj = delegate_flow_obj
		return self.delegate_flow_obj.impl.Load(config_obj, flow_obj)
	}
	return nil
}

func (self *HuntRunnerFlow) Save(
	config_obj *config.Config,
	flow_obj *AFF4FlowObject) error {
	// Store the delegate in our state
	state, err := self.delegate_flow_obj.AsProto()
	if err != nil {
		return err
	}
	flow_obj.SetState(state)
	return nil
}

func (self *HuntRunnerFlow) ProcessMessage(
	config_obj *config.Config,
	flow_obj *AFF4FlowObject,
	message *crypto_proto.GrrMessage) error {
	delegate_err := self.delegate_flow_obj.impl.ProcessMessage(
		config_obj, self.delegate_flow_obj, message)

	// If the delegate flow is no longer running then write its
	// result to the hunt complete queue.
	if delegate_err != nil ||
		self.delegate_flow_obj.FlowContext.State !=
			flows_proto.FlowContext_RUNNING {
		args, err := GetFlowArgs(flow_obj.RunnerArgs)
		if err != nil {
			return err
		}
		hunt_summary_args := args.(*api_proto.HuntInfo)
		utils.Debug(hunt_summary_args)

		urn := hunt_summary_args.HuntId + "/completed/" + hunt_summary_args.ClientId
		db, err := datastore.GetDB(config_obj)
		if err != nil {
			return err
		}

		hunt_summary_args.Result = flow_obj.FlowContext
		serialized_hunt_summary, err := proto.Marshal(hunt_summary_args)
		if err != nil {
			return errors.WithStack(err)
		}
		data := make(map[string][]byte)
		data[constants.HUNTS_SUMMARY_ATTR] = serialized_hunt_summary

		err = db.SetSubjectData(config_obj, urn, 0, data)
		if err != nil {
			return err
		}

		flow_obj.Complete()
	}

	return delegate_err
}

func init() {
	impl := HuntRunnerFlow{}
	default_args, _ := ptypes.MarshalAny(&actions_proto.VQLCollectorArgs{})
	desc := &flows_proto.FlowDescriptor{
		Name:         "HuntRunnerFlow",
		FriendlyName: "HuntRunnerFlow",
		Category:     "Internal",
		Doc:          "Runs a flow as part of a hunt.",
		ArgsType:     "HuntInfo",
		DefaultArgs:  default_args,
	}

	RegisterImplementation(desc, &impl)
}
