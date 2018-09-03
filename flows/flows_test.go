package flows

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	assert "github.com/stretchr/testify/assert"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config "www.velocidex.com/golang/velociraptor/config"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	logging "www.velocidex.com/golang/velociraptor/logging"
)

func TestAFF4FlowObject(t *testing.T) {
	tempdir, err := ioutil.TempDir("", "Velo")
	assert.NoError(t, err)
	defer os.RemoveAll(tempdir)

	config_obj := config.GetDefaultConfig()
	config_obj.Datastore.Implementation = "FileBaseDataStore"
	config_obj.Datastore.Location = tempdir

	runner_args := &flows_proto.FlowRunnerArgs{
		FlowName: "NoSuchFlow",
	}
	_, err = NewAFF4FlowObject(config_obj, runner_args)
	assert.Error(t, err)

	runner_args = &flows_proto.FlowRunnerArgs{
		FlowName: "VInterrogate",
	}

	flow_aff4_obj, err := NewAFF4FlowObject(config_obj, runner_args)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the correct implementation is selected.
	assert.IsType(t, &VInterrogate{}, flow_aff4_obj.impl)

	// Check the initial state is nil
	assert.Nil(t, flow_aff4_obj.flow_state)

	// Make sure the object is not dirty to start with
	assert.Equal(t, flow_aff4_obj.dirty, false)

	some_random_protobuf := &crypto_proto.GrrMessage{
		RequestId: 6666,
	}
	flow_aff4_obj.SetState(some_random_protobuf)
	assert.Equal(t, flow_aff4_obj.dirty, true)
	assert.Equal(t, flow_aff4_obj.flow_state, some_random_protobuf)

	// Now save the object in the database.
	assert.NoError(t,
		SetAFF4FlowObject(config_obj, flow_aff4_obj))

	retrieved_aff4_obj, err := GetAFF4FlowObject(config_obj, flow_aff4_obj.Urn)
	assert.NoError(t, err)

	assert.True(t, proto.Equal(retrieved_aff4_obj.flow_state, flow_aff4_obj.flow_state))
	assert.True(t, proto.Equal(retrieved_aff4_obj.RunnerArgs, flow_aff4_obj.RunnerArgs))
	assert.True(t, proto.Equal(retrieved_aff4_obj.FlowContext, flow_aff4_obj.FlowContext))

	retrieved_aff4_obj.RunnerArgs = nil
	retrieved_aff4_obj.FlowContext = nil
	retrieved_aff4_obj.flow_state = nil
	flow_aff4_obj.RunnerArgs = nil
	flow_aff4_obj.FlowContext = nil
	flow_aff4_obj.flow_state = nil

	assert.Equal(t, retrieved_aff4_obj, flow_aff4_obj)
}

type MyTestFlow struct {
	*BaseFlow
}

func (self *MyTestFlow) New() Flow {
	return &MyTestFlow{&BaseFlow{}}
}

func (self *MyTestFlow) Start(
	config_obj *config.Config,
	flow_obj *AFF4FlowObject,
	args proto.Message) error {

	flow_obj.SetState(&actions_proto.ClientInfo{})

	return nil
}

func (self *MyTestFlow) ProcessMessage(
	config_obj *config.Config,
	flow_obj *AFF4FlowObject,
	message *crypto_proto.GrrMessage) error {

	err := flow_obj.FailIfError(config_obj, message)
	if err != nil {
		return err
	}

	if message.ResponseId == 666 {
		return flow_obj.Complete(config_obj)
	}

	state := flow_obj.GetState().(*actions_proto.ClientInfo)
	state.Info = append(state.Info, &actions_proto.VQLResponse{
		Response: fmt.Sprintf("%d:%d", message.RequestId, message.ResponseId),
	})

	flow_obj.SetState(state)

	return nil
}

// This test ensures the flow runner preserves flow state across
// multiple client POST requests.
func TestFlowRunner(t *testing.T) {
	impl := MyTestFlow{}
	default_args, _ := ptypes.MarshalAny(&empty.Empty{})
	desc := &flows_proto.FlowDescriptor{
		Name:        "MyTestFlow",
		DefaultArgs: default_args,
	}
	RegisterImplementation(desc, &impl)

	tempdir, err := ioutil.TempDir("", "Velo")
	assert.NoError(t, err)
	defer os.RemoveAll(tempdir)

	config_obj := config.GetDefaultConfig()
	config_obj.Datastore.Implementation = "FileBaseDataStore"
	config_obj.Datastore.Location = tempdir

	runner_args := &flows_proto.FlowRunnerArgs{
		FlowName: "MyTestFlow",
		ClientId: "C.0",
	}
	flow_args, err := ptypes.MarshalAny(&empty.Empty{})
	assert.NoError(t, err)
	runner_args.Args = flow_args

	flow_urn, err := StartFlow(config_obj, runner_args)
	assert.NoError(t, err)

	// Check that the flow object is stored properly in the DB.
	flow_aff4_obj, err := GetAFF4FlowObject(config_obj, *flow_urn)
	assert.NoError(t, err)

	// Check that the correct implementation is selected.
	assert.IsType(t, &MyTestFlow{}, flow_aff4_obj.impl)

	// Make some messages - these are replies to Request 1.
	var messages []*crypto_proto.GrrMessage
	for i := uint64(0); i < 3; i++ {
		messages = append(messages, &crypto_proto.GrrMessage{
			RequestId:  1,
			ResponseId: i,
			SessionId:  *flow_urn,
		})
	}

	// Create a runner to receive the messages.
	// Receive the first batch.
	logger := logging.NewLogger(config_obj)
	flow_runner := NewFlowRunner(config_obj, logger)
	flow_runner.ProcessMessages(messages)
	flow_runner.Close()

	flow_aff4_obj, err = GetAFF4FlowObject(config_obj, *flow_urn)
	assert.NoError(t, err)

	state := flow_aff4_obj.GetState().(*actions_proto.ClientInfo).Info
	assert.Equal(t, 3, len(state))

	// A new flow runner to receive another batch of messages.
	flow_runner = NewFlowRunner(config_obj, logger)
	flow_runner.ProcessMessages(messages)
	flow_runner.Close()

	flow_aff4_obj, err = GetAFF4FlowObject(config_obj, *flow_urn)
	assert.NoError(t, err)

	state = flow_aff4_obj.GetState().(*actions_proto.ClientInfo).Info
	assert.Equal(t, 6, len(state))

	// Make sure the flow is still running.
	assert.Equal(t, flow_aff4_obj.FlowContext.State, flows_proto.FlowContext_RUNNING)

	// Send the magic response packet
	message := &crypto_proto.GrrMessage{RequestId: 1, ResponseId: 666, SessionId: *flow_urn}
	flow_runner.ProcessMessages([]*crypto_proto.GrrMessage{message})
	flow_runner.Close()

	flow_aff4_obj, err = GetAFF4FlowObject(config_obj, *flow_urn)
	assert.NoError(t, err)

	// The magic packet should terminate this flow.
	assert.Equal(t, flow_aff4_obj.FlowContext.State, flows_proto.FlowContext_TERMINATED)
}

func TestFlowRunner2(t *testing.T) {
	impl := MyTestFlow{}
	default_args, _ := ptypes.MarshalAny(&empty.Empty{})
	desc := &flows_proto.FlowDescriptor{
		Name:        "MyTestFlow",
		DefaultArgs: default_args,
	}
	RegisterImplementation(desc, &impl)

	tempdir, err := ioutil.TempDir("", "Velo")
	assert.NoError(t, err)
	defer os.RemoveAll(tempdir)

	config_obj := config.GetDefaultConfig()
	config_obj.Datastore.Implementation = "FileBaseDataStore"
	config_obj.Datastore.Location = tempdir

	runner_args := &flows_proto.FlowRunnerArgs{
		FlowName: "MyTestFlow",
		ClientId: "C.0",
	}
	flow_args, err := ptypes.MarshalAny(&empty.Empty{})
	assert.NoError(t, err)
	runner_args.Args = flow_args

	flow_urn, err := StartFlow(config_obj, runner_args)
	assert.NoError(t, err)

	// When our flow gets an client error message it kills the
	// flow. NOTE: This is not necessarily always the case - some
	// flows expect client errors. In this flow the call to
	// FailIfError() will fail the flow if the response to this
	// request is a client error.
	status := &crypto_proto.GrrStatus{
		ErrorMessage: "error",
		Status:       crypto_proto.GrrStatus_GENERIC_ERROR,
	}

	message := &crypto_proto.GrrMessage{
		Type:        crypto_proto.GrrMessage_STATUS,
		ArgsRdfName: "GrrStatus",
		SessionId:   *flow_urn,
	}
	message.Args, err = proto.Marshal(status)
	assert.NoError(t, err)

	// When the flow receives a client error, it should store the
	// error in the flow context.
	logger := logging.NewLogger(config_obj)
	flow_runner := NewFlowRunner(config_obj, logger)
	flow_runner.ProcessMessages([]*crypto_proto.GrrMessage{message})
	flow_runner.Close()

	flow_aff4_obj, err := GetAFF4FlowObject(config_obj, *flow_urn)
	assert.NoError(t, err)

	assert.Equal(t, flow_aff4_obj.FlowContext.State, flows_proto.FlowContext_ERROR)
	assert.Equal(t, flow_aff4_obj.FlowContext.Status, "error")
}
