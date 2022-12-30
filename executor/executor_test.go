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
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

type ExecutorTestSuite struct {
	test_utils.TestSuite
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

	// Send two requests for the same flow in parallel.
	flow_id := fmt.Sprintf("F.XXX%d", utils.GetId())
	for i := 0; i < 2; i++ {
		executor.Inbound <- &crypto_proto.VeloMessage{
			AuthState: crypto_proto.VeloMessage_AUTHENTICATED,
			SessionId: flow_id,
			VQLClientAction: &actions_proto.VQLCollectorArgs{
				Query: []*actions_proto.VQLRequest{
					{VQL: "SELECT 'a' FROM scope()"},
				},
			},
			RequestId: 1}
	}

	// collect the log messages and ensure the log id is sequential
	// across all requests.
	log_messages := []*crypto_proto.LogMessage{}
	log_ids := []int64{}

	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		mu.Lock()
		defer mu.Unlock()

		log_messages = nil
		log_ids = nil

		for _, msg := range received_messages {
			if msg.LogMessage != nil {
				log_messages = append(log_messages, msg.LogMessage)
				log_ids = append(log_ids, msg.LogMessage.Id)
			}
		}

		return len(log_messages) == 6
	})

	assert.Equal(self.T(), []int64{1, 2, 3, 4, 5, 6}, log_ids)
}

func TestExecutorTestSuite(t *testing.T) {
	suite.Run(t, new(ExecutorTestSuite))
}
