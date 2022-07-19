package executor

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

type ExecutorTestSuite struct {
	suite.Suite
}

// Cancelling the executor multiple times will cause a single
// cancellation state and then ignore the rest.
func (self *ExecutorTestSuite) TestCancellation() {
	t := self.T()

	// Max time for deadlock detection.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wg := &sync.WaitGroup{}
	config_obj := config.GetDefaultConfig()
	executor, err := NewClientExecutor(ctx, "", config_obj)
	require.NoError(t, err)

	var received_messages []*crypto_proto.VeloMessage

	wg.Add(1)
	go func() {
		defer wg.Done()

		// Wait here until the executor is fully cancelled.
		for {
			select {
			case <-ctx.Done():
				return

			case message := <-executor.Outbound:
				received_messages = append(
					received_messages, message)
			}
		}
	}()

	// Send cancel message
	flow_id := "F.XXX"
	for i := 0; i < 100; i++ {
		executor.Inbound <- &crypto_proto.VeloMessage{
			AuthState: crypto_proto.VeloMessage_AUTHENTICATED,
			SessionId: flow_id,
			Cancel:    &crypto_proto.Cancel{},
			RequestId: 1}
	}

	// Tear down the executor and wait for it to finish.
	cancel()

	wg.Wait()

	// The cancel message should generate 1 log + a status
	// message. This should only be done once, no matter how many
	// cancellations are sent.
	require.Equal(t, len(received_messages), 2)
	require.NotNil(t, received_messages[0].LogMessage)
	require.NotNil(t, received_messages[1].Status)
}

func TestExecutorTestSuite(t *testing.T) {
	suite.Run(t, new(ExecutorTestSuite))
}
