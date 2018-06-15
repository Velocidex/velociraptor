package flows

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/golang/protobuf/proto"
	"path"
	"strings"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/responder"
	utils "www.velocidex.com/golang/velociraptor/testing"
)

var (
	implementations map[string]Flow
)

// The Flow runner processes a sequence of packets.
type FlowRunner struct {
	config     *config.Config
	db         datastore.DataStore
	flow_cache map[string]*AFF4FlowObject
}

func (self *FlowRunner) getFlow(flow_urn string) (*AFF4FlowObject, error) {
	cached_flow, pres := self.flow_cache[flow_urn]
	if !pres {
		cached_flow, err := GetAFF4FlowObject(
			self.config, flow_urn)
		if err != nil {
			return nil, err
		}

		self.flow_cache[flow_urn] = cached_flow
		return cached_flow, nil
	}
	return cached_flow, nil
}

func (self *FlowRunner) ProcessMessages(messages []*crypto_proto.GrrMessage) {
	for _, message := range messages {
		cached_flow, err := self.getFlow(message.SessionId)
		if err != nil {
			utils.Debug(err)
			continue
		}
		err = cached_flow.impl.ProcessMessage(
			self.config, cached_flow, message)
		if err != nil {
			utils.Debug(err)
		}
	}
}

// Flush all the cached flows back to the DB.
func (self *FlowRunner) Close() {
	for urn, cached_flow := range self.flow_cache {
		if !cached_flow.dirty {
			continue
		}

		err := SetAFF4FlowObject(
			self.config, cached_flow, urn)
		if err != nil {
			utils.Debug(err)
		}
	}

}

func NewFlowRunner(config *config.Config, db datastore.DataStore) *FlowRunner {
	result := FlowRunner{
		config:     config,
		db:         db,
		flow_cache: make(map[string]*AFF4FlowObject),
	}
	return &result
}

// Flows are factories which have no internal state. They must be
// thread safe and reusable multiple times.
type Flow interface {
	Start(
		config *config.Config,
		flow_runner_args *flows_proto.FlowRunnerArgs,
	) (*string, error)
	ProcessMessage(
		config *config.Config,
		flow_obj *AFF4FlowObject,
		message *crypto_proto.GrrMessage) error
}

// The AFF4 object contains the state of the flow.
type AFF4FlowObject struct {
	// Will be set to true if the state needs to be flushed.
	dirty bool

	config       *config.Config
	runner_args  *flows_proto.FlowRunnerArgs
	flow_context *flows_proto.FlowContext
	flow_state   map[string]interface{}

	// The flow implementation has no internal state and uses this
	// object to contain the flow's state.
	impl Flow
}

// Fails the flow if an error occured
func (self *AFF4FlowObject) FailIfError(message *crypto_proto.GrrMessage) error {
	utils.Debug(message)
	if message.Type == crypto_proto.GrrMessage_STATUS {
		status, ok := responder.ExtractGrrMessagePayload(
			message).(*crypto_proto.GrrStatus)
		if ok {
			utils.Debug(status)
			self.flow_context.State = flows_proto.FlowContext_ERROR
			self.flow_context.Status = status.ErrorMessage
			self.flow_context.Backtrace = status.Backtrace
			self.dirty = true

			return errors.New(status.ErrorMessage)
		}
	}
	return nil
}

func (self *AFF4FlowObject) SetState(key string, value interface{}) {
	self.dirty = true
	self.flow_state[key] = value
}

func NewAFF4FlowObject(
	config *config.Config,
	runner_args *flows_proto.FlowRunnerArgs) (*AFF4FlowObject, error) {
	result := AFF4FlowObject{
		config:       config,
		runner_args:  runner_args,
		flow_context: new(flows_proto.FlowContext),
	}
	result.flow_state = make(map[string]interface{})

	// Attach the implementation.
	impl, ok := GetImpl(runner_args.FlowName)
	if !ok {
		return nil, errors.New(fmt.Sprintf(
			"Unknown flow %s", runner_args.FlowName))
	}

	result.impl = impl

	return &result, nil
}

func GetAFF4FlowObject(
	config_obj *config.Config,
	flow_urn string) (*AFF4FlowObject, error) {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	data, err := db.GetSubjectData(config_obj, flow_urn)
	if err != nil {
		return nil, err
	}

	flow_runner_args := &flows_proto.FlowRunnerArgs{}
	serialized_flow_runner_arg, pres := data[constants.FLOW_RUNNER_ARGS]
	if pres {
		err := proto.Unmarshal(
			serialized_flow_runner_arg, flow_runner_args)
		if err != nil {
			return nil, err
		}
	}

	result, err := NewAFF4FlowObject(config_obj, flow_runner_args)
	if err != nil {
		return nil, err
	}

	serialized_state, pres := data[constants.FLOW_STATE]
	if pres {
		err := json.Unmarshal(
			serialized_state, &result.flow_state)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

func SetAFF4FlowObject(
	config_obj *config.Config,
	obj *AFF4FlowObject,
	urn string) error {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	data := make(map[string][]byte)
	if obj.runner_args == nil {
		return errors.New("Flow runner must be populated.")
	}

	value, err := proto.Marshal(obj.runner_args)
	if err != nil {
		return err
	}
	data[constants.FLOW_RUNNER_ARGS] = value

	if obj.flow_context == nil {
		return errors.New("Flow context must be populated.")
	}

	value, err = proto.Marshal(obj.flow_context)
	if err != nil {
		return err
	}

	data[constants.FLOW_CONTEXT] = value

	// This is used for backwards compatibility with GRR's GUI.
	data[constants.AFF4_TYPE] = []byte("GRRFlow")

	// Serialize the state into the database.
	value, err = json.Marshal(obj.flow_state)
	if err != nil {
		return err
	}
	data[constants.FLOW_STATE] = value

	err = db.SetSubjectData(config_obj, urn, data)
	if err != nil {
		return err
	}

	data = make(map[string][]byte)
	dir, name := path.Split(urn)
	index_predicate := "index:dir/" + name
	data[index_predicate] = []byte("X")
	err = db.SetSubjectData(config_obj, strings.TrimRight(dir, "/"), data)
	if err != nil {
		return err
	}

	return nil
}

func RegisterImplementation(name string, impl Flow) {
	if implementations == nil {
		implementations = make(map[string]Flow)
	}

	implementations[name] = impl
}

func GetImpl(name string) (Flow, bool) {
	result, pres := implementations[name]
	return result, pres
}

func StartFlow(
	config_obj *config.Config,
	flow_runner_args *flows_proto.FlowRunnerArgs) (*string, error) {
	if flow_runner_args.FlowName == "" {
		return nil, errors.New("No flow name")
	}

	flow_obj, err := NewAFF4FlowObject(config_obj, flow_runner_args)
	if err != nil {
		return nil, err
	}

	flow_id, err := flow_obj.impl.Start(config_obj, flow_obj.runner_args)
	if err != nil {
		return nil, err
	}

	err = SetAFF4FlowObject(config_obj, flow_obj, *flow_id)
	if err != nil {
		return nil, err
	}

	return flow_id, nil
}

func getNewFlowId(client_id string) string {
	result := make([]byte, 8)
	buf := make([]byte, 4)

	rand.Read(buf)
	hex.Encode(result, buf)

	return fmt.Sprintf("aff4:/%s/flows/E:%s", client_id, string(result))
}
