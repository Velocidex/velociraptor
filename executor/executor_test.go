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
	"www.velocidex.com/golang/velociraptor/utils"
)

type ExecutorTestSuite struct {
	suite.Suite
}

func (self *ExecutorTestSuite) TestCancellation() {
	t := self.T()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	config_obj := config.GetDefaultConfig()
	executor, err := NewClientExecutor(ctx, config_obj)
	require.NoError(t, err)

	wg := &sync.WaitGroup{}

	var received_messages []*crypto_proto.GrrMessage

	wg.Add(1)
	go func() {
		defer wg.Done()

		for message := range executor.Outbound {
			utils.Debug(message)
			received_messages = append(
				received_messages, message)
		}
	}()

	// Send cancel message
	flow_id := "F.XXX"
	executor.Inbound <- &crypto_proto.GrrMessage{
		AuthState: crypto_proto.GrrMessage_AUTHENTICATED,
		SessionId: flow_id,
		Cancel:    &crypto_proto.Cancel{},
		RequestId: 1}

	executor.Inbound <- &crypto_proto.GrrMessage{
		AuthState: crypto_proto.GrrMessage_AUTHENTICATED,
		SessionId: flow_id,
		Cancel:    &crypto_proto.Cancel{},
		RequestId: 1}

	wg.Wait()
}

func TestExecutorTestSuite(t *testing.T) {
	suite.Run(t, new(ExecutorTestSuite))
}
