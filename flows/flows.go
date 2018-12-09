package flows

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	errors "github.com/pkg/errors"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/responder"
	urns "www.velocidex.com/golang/velociraptor/urns"
	users "www.velocidex.com/golang/velociraptor/users"
)

var (
	implementations map[string]FlowImplementation
)

type FlowImplementation struct {
	flow       Flow
	descriptor *flows_proto.FlowDescriptor
}

// The Flow runner processes a sequence of packets.
type FlowRunner struct {
	config     *api_proto.Config
	flow_cache map[string]*AFF4FlowObject
	logger     *logging.Logger
}

func (self *FlowRunner) getFlow(flow_urn string) (*AFF4FlowObject, error) {
	cached_flow, pres := self.flow_cache[flow_urn]
	if !pres {
		cached_flow, err := GetAFF4FlowObject(self.config, flow_urn)
		if err != nil {
			return nil, err
		}

		self.flow_cache[flow_urn] = cached_flow
		cached_flow.impl.Load(self.config, cached_flow)

		return cached_flow, nil
	}
	return cached_flow, nil
}

func (self *FlowRunner) ProcessMessages(messages []*crypto_proto.GrrMessage) {
	var message *crypto_proto.GrrMessage

	defer func() {
		if r := recover(); r != nil {
			self.logger.Error(
				fmt.Sprintf(
					"%v, during processing of message %v",
					r, message), errors.New("Panic"))
		}
	}()

	for _, message = range messages {
		cached_flow, err := self.getFlow(message.SessionId)
		if err != nil {
			self.logger.Error(fmt.Sprintf("FlowRunner %v: ", message), err)
			continue
		}

		// Do not feed messages to flows that are terminated,
		// just drop these on the floor.
		if cached_flow.FlowContext != nil &&
			cached_flow.FlowContext.State != flows_proto.FlowContext_RUNNING {
			continue
		}

		// Handle log messages automatically so flows do not
		// need to all remember to do this.
		if message.RequestId == constants.LOG_SINK {
			cached_flow.LogMessage(message)
			continue
		}

		err = cached_flow.impl.ProcessMessage(
			self.config, cached_flow, message)
		if err != nil {
			if cached_flow.FlowContext != nil {
				cached_flow.FlowContext.State = flows_proto.FlowContext_ERROR
				cached_flow.FlowContext.Status = err.Error()
				cached_flow.FlowContext.Backtrace = ""
				cached_flow.dirty = true
			}
			self.logger.Error("FlowRunner", err)
			return
		}
	}
}

// Flush all the cached flows back to the DB.
func (self *FlowRunner) Close() {
	for _, cached_flow := range self.flow_cache {
		if !cached_flow.dirty {
			continue
		}
		cached_flow.impl.Save(self.config, cached_flow)
		err := SetAFF4FlowObject(self.config, cached_flow)
		if err != nil {
			self.logger.Error("FlowRunner", err)
		}
	}
}

func NewFlowRunner(config *api_proto.Config, logger *logging.Logger) *FlowRunner {
	result := FlowRunner{
		config:     config,
		logger:     logger,
		flow_cache: make(map[string]*AFF4FlowObject),
	}
	return &result
}

// Flows are factories which have no persistent internal state. They
// must be thread safe and reusable multiple times. The flow runner
// uses flows in a predicatable cycle:

// When a set of messages arrive at the server from the client (e.g. a
// packet sent via a POST request), the runner makes a copy of the
// Flow object and calls its Load() method. This gives the flow an
// opportunity to reset itself from the stored "state" object.

// The runner then processes each message destined for the flow in
// turn through the ProcessMessage() method.

// After the runner completes all the messages in this packet, the
// runner calls Save() method and then the flow is destroyed. The flow
// is responsible for loading and saving its internal state from
// persistant state using the GetState() and SetState()
// functions. Note that during the life of the flow (i.e. from Start()
// until Complete()), the flow may receive multiple packets and
// therefore should store its state reliably.

// Some flows require no persistant state and therefore should have
// empty Load() and Save() methods. These flows will run faster and be
// more efficient. You can get that by embedding the BaseFlow type.
type Flow interface {
	Start(
		config *api_proto.Config,
		flow_obj *AFF4FlowObject,
		args proto.Message,
	) error

	// This method is called by the runner prior to processing
	// messages.
	Load(config_obj *api_proto.Config, flow_obj *AFF4FlowObject) error

	// Each message is processed with this method. Note that
	// messages may be split across client POST operations. The
	// flow runner is responsible for saving and restoring the
	// flow's state, so if the flow requires to maintain state
	// across POST operations, they should store this state inside
	// the flow_obj.SetState().
	ProcessMessage(
		config *api_proto.Config,
		flow_obj *AFF4FlowObject,
		message *crypto_proto.GrrMessage) error

	// This method is called by the runner after processing
	// messages and before the flow is destroyed.
	Save(config_obj *api_proto.Config, flow_obj *AFF4FlowObject) error

	// Create a new flow of this type.
	New() Flow
}

// The AFF4 object contains the state of the flow.
type AFF4FlowObject struct {
	// Will be set to true if the state needs to be flushed.
	dirty bool

	Urn         string
	RunnerArgs  *flows_proto.FlowRunnerArgs
	FlowContext *flows_proto.FlowContext

	// An opaque place for flows to store state. If the flow wants
	// to use the state they can set it in Start() and the runner
	// will ensure it gets serialized and unserialized when
	// required.
	flow_state proto.Message

	// The flow implementation has no internal state and uses this
	// object to contain the flow's state.
	impl Flow
}

func (self *AFF4FlowObject) SetState(value proto.Message) {
	self.dirty = true
	self.flow_state = value
}

func (self *AFF4FlowObject) SetContext(value *flows_proto.FlowContext) {
	self.dirty = true
	self.FlowContext = value
}

func (self *AFF4FlowObject) GetState() proto.Message {
	return self.flow_state
}

func (self *AFF4FlowObject) AsProto() (*flows_proto.AFF4FlowObject, error) {
	result := &flows_proto.AFF4FlowObject{
		Urn:         self.Urn,
		RunnerArgs:  self.RunnerArgs,
		FlowContext: self.FlowContext,
	}

	if self.flow_state != nil {
		any_state, err := ptypes.MarshalAny(self.flow_state)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		result.FlowState = any_state
	}
	return result, nil
}

func AFF4FlowObjectFromProto(aff4_flow_obj_proto *flows_proto.AFF4FlowObject) (
	*AFF4FlowObject, error) {

	if aff4_flow_obj_proto.Urn == "" ||
		aff4_flow_obj_proto.RunnerArgs == nil ||
		aff4_flow_obj_proto.FlowContext == nil {
		return nil, errors.New(
			fmt.Sprintf("Invalid AFF4FlowObject protobuf (%v).",
				aff4_flow_obj_proto))
	}

	result := &AFF4FlowObject{
		dirty:       false,
		Urn:         aff4_flow_obj_proto.Urn,
		RunnerArgs:  aff4_flow_obj_proto.RunnerArgs,
		FlowContext: aff4_flow_obj_proto.FlowContext,
	}

	if result.FlowContext == nil {
		result.FlowContext = &flows_proto.FlowContext{}
	}

	if aff4_flow_obj_proto.FlowState != nil {
		var state ptypes.DynamicAny
		err := ptypes.UnmarshalAny(aff4_flow_obj_proto.FlowState, &state)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		result.flow_state = state.Message
	}

	impl, ok := GetImpl(result.RunnerArgs.FlowName)
	if !ok {
		return nil, errors.New(fmt.Sprintf(
			"Unknown flow %s", result.RunnerArgs.FlowName))
	}

	result.impl = impl

	return result, nil
}

// Complete the flow.
func (self *AFF4FlowObject) Complete(config_obj *api_proto.Config) error {
	self.dirty = true
	self.FlowContext.State = flows_proto.FlowContext_TERMINATED
	self.FlowContext.KillTimestamp = uint64(time.Now().UnixNano() / 1000)
	self.flow_state = nil

	// Notify to our user if we need to.
	if self.RunnerArgs.NotifyToUser && self.RunnerArgs.Creator != "" {
		err := users.Notify(
			config_obj,
			&api_proto.UserNotification{
				Username: self.RunnerArgs.Creator,
				NotificationType: api_proto.
					UserNotification_TYPE_FLOW_RUN_COMPLETED,
				State:     api_proto.UserNotification_STATE_PENDING,
				Timestamp: uint64(time.Now().UTC().UnixNano() / 1000),
				Message: fmt.Sprintf("Flow %s completed successfully.",
					self.RunnerArgs.FlowName),
				Reference: &api_proto.ObjectReference{
					Union: &api_proto.ObjectReference_Flow{
						Flow: &api_proto.FlowReference{
							FlowId:   path.Base(self.Urn),
							ClientId: self.RunnerArgs.ClientId,
						},
					},
				},
			})
		if err != nil {
			return err
		}
	}
	return nil
}

// Fails the flow if an error occured and copy the client's backtrace
// to the flow.
func (self *AFF4FlowObject) FailIfError(
	config_obj *api_proto.Config,
	message *crypto_proto.GrrMessage) error {
	if message.Type == crypto_proto.GrrMessage_STATUS {
		status, ok := responder.ExtractGrrMessagePayload(
			message).(*crypto_proto.GrrStatus)
		if ok {
			// If the status is OK then we do not fail the flow.
			if status.Status == crypto_proto.GrrStatus_OK {
				return nil
			}

			self.FlowContext.State = flows_proto.FlowContext_ERROR
			self.FlowContext.Status = status.ErrorMessage
			self.FlowContext.Backtrace = status.Backtrace
			self.dirty = true

			return errors.New(status.ErrorMessage)
		}
	}

	// Notify to our user if we need to.
	if self.RunnerArgs.NotifyToUser && self.RunnerArgs.Creator != "" {
		err := users.Notify(
			config_obj,
			&api_proto.UserNotification{
				Username: self.RunnerArgs.Creator,
				NotificationType: api_proto.
					UserNotification_TYPE_FLOW_RUN_FAILED,
				State:     api_proto.UserNotification_STATE_PENDING,
				Timestamp: uint64(time.Now().UTC().UnixNano() / 1000),
				Message: fmt.Sprintf("Flow %s failed!.",
					self.RunnerArgs.FlowName),
				Reference: &api_proto.ObjectReference{
					Union: &api_proto.ObjectReference_Flow{
						Flow: &api_proto.FlowReference{
							FlowId:   path.Base(self.Urn),
							ClientId: self.RunnerArgs.ClientId,
						},
					},
				},
			})
		if err != nil {
			return err
		}
	}

	return nil
}

// Checks if the message represents the last response to the request.
func (self *AFF4FlowObject) IsRequestComplete(message *crypto_proto.GrrMessage) bool {
	return message.Type == crypto_proto.GrrMessage_STATUS
}

func (self *AFF4FlowObject) Log(log_msg string) {
	self.FlowContext.Logs = append(
		self.FlowContext.Logs, &crypto_proto.LogMessage{
			Message:   log_msg,
			Timestamp: uint64(time.Now().UTC().UnixNano() / 1000),
		})
	self.dirty = true
}

func (self *AFF4FlowObject) LogMessage(message *crypto_proto.GrrMessage) {
	log_msg, ok := responder.ExtractGrrMessagePayload(
		message).(*crypto_proto.LogMessage)
	if ok {
		self.FlowContext.Logs = append(self.FlowContext.Logs, log_msg)
		self.dirty = true
	}
}

func NewAFF4FlowObject(
	config *api_proto.Config,
	runner_args *flows_proto.FlowRunnerArgs) (*AFF4FlowObject, error) {
	result := AFF4FlowObject{
		Urn:        GetNewFlowIdForClient(runner_args.ClientId),
		RunnerArgs: runner_args,
		FlowContext: &flows_proto.FlowContext{
			State:      flows_proto.FlowContext_RUNNING,
			CreateTime: uint64(time.Now().UnixNano() / 1000),
		},
	}

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
	config_obj *api_proto.Config,
	flow_urn string) (*AFF4FlowObject, error) {

	// Handle well known flows. Well known flows are not
	// serialized to the DataStore because they have no internal
	// state or any args.
	switch flow_urn {
	case constants.FOREMAN_WELL_KNOWN_FLOW:
		return &AFF4FlowObject{
			impl: &Foreman{},
		}, nil
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	flow_obj := &flows_proto.AFF4FlowObject{}
	err = db.GetSubject(config_obj, flow_urn, flow_obj)
	if err != nil {
		return nil, err
	}

	return AFF4FlowObjectFromProto(flow_obj)
}

func SetAFF4FlowObject(
	config_obj *api_proto.Config,
	obj *AFF4FlowObject) error {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	if obj.RunnerArgs == nil {
		return errors.New("Flow runner must be populated.")
	}

	if obj.FlowContext == nil {
		return errors.New("Flow context must be populated.")
	}

	obj.FlowContext.ActiveTime = uint64(time.Now().UnixNano() / 1000)
	flow_obj, err := obj.AsProto()
	if err != nil {
		return err
	}

	// The object is not dirty any more.
	obj.dirty = false

	return db.SetSubject(config_obj, flow_obj.Urn, flow_obj)
}

func RegisterImplementation(descriptor *flows_proto.FlowDescriptor, impl Flow) {
	if implementations == nil {
		implementations = make(map[string]FlowImplementation)
	}

	if descriptor.DefaultArgs == nil {
		panic(fmt.Sprintf("Flow %s does not specify a default arg.",
			descriptor.Name))
	}

	implementations[descriptor.Name] = FlowImplementation{
		flow:       impl,
		descriptor: descriptor,
	}
}

func GetImpl(name string) (Flow, bool) {
	result, pres := implementations[name]
	if !pres {
		return nil, false

	}
	return result.flow.New(), pres
}

func GetDescriptors() []*flows_proto.FlowDescriptor {
	var result []*flows_proto.FlowDescriptor
	for _, item := range implementations {
		result = append(result, item.descriptor)
	}

	return result
}

func GetFlowArgs(flow_runner_args *flows_proto.FlowRunnerArgs) (proto.Message, error) {
	// Return default args
	if flow_runner_args.Args == nil {
		for _, desc := range GetDescriptors() {
			if desc.Name == flow_runner_args.FlowName {
				return desc.DefaultArgs, nil
			}
		}
	}

	// Decode args from flow runner args.
	var tmp_args ptypes.DynamicAny
	err := ptypes.UnmarshalAny(flow_runner_args.Args, &tmp_args)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return tmp_args.Message, nil
}

func StartFlow(
	config_obj *api_proto.Config,
	flow_runner_args *flows_proto.FlowRunnerArgs) (*string, error) {
	if flow_runner_args.StartTime == 0 {
		flow_runner_args.StartTime = uint64(time.Now().UnixNano() / 1000)
	}

	if flow_runner_args.ClientId == "" {
		return nil, errors.New("Client id not provided.")
	}

	if flow_runner_args.FlowName == "" {
		return nil, errors.New("No flow name")
	}

	flow_obj, err := NewAFF4FlowObject(config_obj, flow_runner_args)
	if err != nil {
		return nil, err
	}

	args, err := GetFlowArgs(flow_runner_args)
	if err != nil {
		return nil, err
	}

	err = flow_obj.impl.Start(config_obj, flow_obj, args)
	if err != nil {
		return nil, err
	}

	err = flow_obj.impl.Save(config_obj, flow_obj)
	if err != nil {
		return nil, err
	}

	err = SetAFF4FlowObject(config_obj, flow_obj)
	if err != nil {
		return nil, err
	}

	return &flow_obj.Urn, nil
}

func GetNewFlowIdForClient(client_id string) string {
	result := make([]byte, 8)
	buf := make([]byte, 4)

	rand.Read(buf)
	hex.Encode(result, buf)

	return urns.BuildURN(
		"clients", client_id,
		"flows", constants.FLOW_PREFIX+string(result))
}

func StoreResultInFlow(
	config_obj *api_proto.Config,
	flow_obj *AFF4FlowObject,
	message *crypto_proto.GrrMessage) error {

	next_result_id := flow_obj.FlowContext.TotalResults
	flow_obj.FlowContext.TotalResults += 1
	flow_obj.dirty = true

	urn := fmt.Sprintf("%s/results/%d", flow_obj.Urn, next_result_id)
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}
	err = db.SetSubject(config_obj, urn, message)
	if err != nil {
		return err
	}

	return nil
}

type BaseFlow struct{}

func (self *BaseFlow) Start(
	config *api_proto.Config,
	flow_obj *AFF4FlowObject,
	args proto.Message) error {
	return errors.New("Unable to start flow directly")
}

func (self *BaseFlow) Load(config_obj *api_proto.Config, flow_obj *AFF4FlowObject) error {
	return nil
}

func (self *BaseFlow) Save(config_obj *api_proto.Config, flow_obj *AFF4FlowObject) error {
	return nil
}
