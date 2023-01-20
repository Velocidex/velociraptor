package executor

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/config"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

type ExecutorTestSuite struct {
	test_utils.TestSuite
}

func (self *ExecutorTestSuite) SetupTest() {
	self.TestSuite.SetupTest()

	err := self.Sm.Start(responder.StartFlowManager)
	assert.NoError(self.T(), err)
}

// Cancelling the executor multiple times will cause a single
// cancellation state and then ignore the rest.
func (self *ExecutorTestSuite) TestCancellation() {
	t := self.T()

	// Max time for deadlock detection.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	config_obj := config.GetDefaultConfig()
	executor, err := NewClientExecutor(ctx, "", config_obj)
	require.NoError(t, err)

	var mu sync.Mutex
	var received_messages []*crypto_proto.VeloMessage

	// Collect outbound messages
	go func() {
		for {
			select {
			// Wait here until the executor is fully cancelled.
			case <-ctx.Done():
				return

			case message := <-executor.Outbound:
				mu.Lock()
				received_messages = append(
					received_messages, message)
				mu.Unlock()
			}
		}
	}()

	// Send many cancel messages
	flow_id := fmt.Sprintf("F.XXX%d", utils.GetId())
	for i := 0; i < 10; i++ {
		executor.Inbound <- &crypto_proto.VeloMessage{
			AuthState: crypto_proto.VeloMessage_AUTHENTICATED,
			SessionId: flow_id,
			Cancel:    &crypto_proto.Cancel{},
			RequestId: 1}
	}

	// The cancel message should generate 1 log + a status
	// message. This should only be done once, no matter how many
	// cancellations are sent.
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		mu.Lock()
		defer mu.Unlock()

		return len(received_messages) == 2
	})

	mu.Lock()
	defer mu.Unlock()

	require.Equal(t, len(received_messages), 2)
	require.NotNil(t, received_messages[0].LogMessage)
	require.NotNil(t, received_messages[1].Status)
}

// Cancelling the executor multiple times will cause a single
// cancellation state and then ignore the rest.
func (self *ExecutorTestSuite) TestLogMessages() {
	t := self.T()

	// Max time for deadlock detection.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	config_obj := config.GetDefaultConfig()
	executor, err := NewClientExecutor(ctx, "", config_obj)
	require.NoError(t, err)

	var mu sync.Mutex
	var received_messages []*crypto_proto.VeloMessage

	// Collect outbound messages
	go func() {
		for {
			select {
			// Wait here until the executor is fully cancelled.
			case <-ctx.Done():
				return

			case message := <-executor.Outbound:
				mu.Lock()
				received_messages = append(
					received_messages, message)
				mu.Unlock()
			}
		}
	}()

	// Send two requests for the same flow in parallel these should
	// generate a bunch of log messages.
	flow_id := fmt.Sprintf("F.XXX%d", utils.GetId())
	executor.Inbound <- &crypto_proto.VeloMessage{
		AuthState: crypto_proto.VeloMessage_AUTHENTICATED,
		SessionId: flow_id,
		FlowRequest: &crypto_proto.FlowRequest{
			VQLClientActions: []*actions_proto.VQLCollectorArgs{{
				Query: []*actions_proto.VQLRequest{
					// Log 100 messages
					{VQL: "SELECT log(message='log %v', args=count()) FROM range(end=10)"},
				}}},
		},
		RequestId: 1}

	// collect the log messages and ensure they are all batched in one response.
	log_messages := []*crypto_proto.LogMessage{}

	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		mu.Lock()
		defer mu.Unlock()

		var total_messages uint64
		log_messages = nil

		for _, msg := range received_messages {
			if msg.LogMessage != nil {
				log_messages = append(log_messages, msg.LogMessage)

				// Each log message should have its Id field the equal
				// to the next expected row.
				assert.Equal(self.T(), msg.LogMessage.Id, int64(total_messages))
				total_messages += msg.LogMessage.NumberOfRows
			}
		}

		return total_messages > 10
	})

	// Log messages should be combined into few messages.
	assert.True(self.T(), len(log_messages) <= 2, "Too many log messages")
}

func TestExecutorTestSuite(t *testing.T) {
	suite.Run(t, new(ExecutorTestSuite))
}
