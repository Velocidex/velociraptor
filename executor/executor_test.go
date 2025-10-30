package executor

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/flows"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"

	_ "www.velocidex.com/golang/velociraptor/accessors/data"
)

type MessageCollector struct {
	mu                sync.Mutex
	received_messages []*crypto_proto.VeloMessage
}

func (self *MessageCollector) Messages() []*crypto_proto.VeloMessage {
	self.mu.Lock()
	defer self.mu.Unlock()

	return append([]*crypto_proto.VeloMessage{}, self.received_messages...)
}

func NewMessageCollector(ctx context.Context, executor *ClientExecutor) *MessageCollector {
	self := &MessageCollector{}

	// Collect outbound messages
	go func() {
		for {
			select {
			// Wait here until the executor is fully cancelled.
			case <-ctx.Done():
				return

			case message := <-executor.Outbound:
				self.mu.Lock()
				self.received_messages = append(
					self.received_messages, message)
				self.mu.Unlock()
			}
		}
	}()

	return self
}

type ExecutorTestSuite struct {
	test_utils.TestSuite

	client_id string
}

func (self *ExecutorTestSuite) SetupTest() {
	self.TestSuite.SetupTest()

	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	err = client_info_manager.Set(self.Ctx, &services.ClientInfo{
		&actions_proto.ClientInfo{
			ClientId: self.client_id,
		}})
	assert.NoError(self.T(), err)
}

// Cancelling the flow multiple times will cause a single
// cancellation state and then ignore the rest.
func (self *ExecutorTestSuite) TestCancellation() {
	t := self.T()

	// Max time for deadlock detection.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()

	config_obj := config.GetDefaultConfig()
	executor, err := NewClientExecutor(ctx, "", config_obj)
	require.NoError(t, err)

	flow_id := fmt.Sprintf("F.XXX%d", utils.GetId())

	actions.QueryLog.Clear()

	collector := NewMessageCollector(ctx, executor)

	// Send the executor a flow request
	executor.Inbound <- &crypto_proto.VeloMessage{
		AuthState: crypto_proto.VeloMessage_AUTHENTICATED,
		SessionId: flow_id,
		FlowRequest: &crypto_proto.FlowRequest{
			VQLClientActions: []*actions_proto.VQLCollectorArgs{{
				Query: []*actions_proto.VQLRequest{
					{
						Name: "Query",
						VQL:  "SELECT sleep(time=1000) FROM scope()",
					},
				},
			}},
		},
	}

	// Wait until the query starts running.
	vtesting.WaitUntil(time.Second*50, self.T(), func() bool {
		return len(actions.QueryLog.Get()) > 0
	})

	cancel_message := &crypto_proto.VeloMessage{
		AuthState: crypto_proto.VeloMessage_AUTHENTICATED,
		SessionId: flow_id,
		Cancel:    &crypto_proto.Cancel{},
		RequestId: 1}

	// Send many cancel messages
	for i := 0; i < 10; i++ {
		executor.Inbound <- cancel_message
	}

	// The cancel message should generate 1 log + a status
	// message. This should only be done once, no matter how many
	// cancellations are sent.
	vtesting.WaitUntil(time.Second*5, self.T(), func() bool {
		received_messages := collector.Messages()

		// Should be at least one stat message and several log messages
		stats := getFlowStat(received_messages)
		if stats == nil || stats.FlowStats == nil ||
			len(stats.FlowStats.QueryStatus) == 0 {
			return false
		}

		return stats.FlowStats.FlowComplete
	})

	received_messages := collector.Messages()

	// An error status
	stats := getFlowStat(received_messages)
	assert.Equal(self.T(), crypto_proto.VeloStatus_GENERIC_ERROR,
		stats.FlowStats.QueryStatus[0].Status)
	assert.Contains(self.T(), getLogMessages(received_messages),
		"Cancelled all inflight queries")
}

// Exceeding flow upload limit will cancel the flow
func (self *ExecutorTestSuite) TestUploadCancellation() {
	t := self.T()

	// Max time for deadlock detection.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()

	config_obj := config.GetDefaultConfig()
	executor, err := NewClientExecutor(ctx, "", config_obj)
	require.NoError(t, err)

	flow_id := "F.TestUploadCancellation"

	actions.QueryLog.Clear()

	collector := NewMessageCollector(ctx, executor)

	// Send the executor a flow request
	executor.Inbound <- &crypto_proto.VeloMessage{
		AuthState: crypto_proto.VeloMessage_AUTHENTICATED,
		SessionId: flow_id,
		FlowRequest: &crypto_proto.FlowRequest{
			// Only 10 bytes are allowed.
			MaxUploadBytes: 10,
			VQLClientActions: []*actions_proto.VQLCollectorArgs{{
				Query: []*actions_proto.VQLRequest{
					{
						Name: "Query",
						VQL: `
        SELECT upload(accessor="data",
                      file="This is a long test with many letters",
                      name=format(format="File%v.txt", args=_value)) AS Upload
        FROM range(end=10)`,
					},
				},
			}},
		},
	}

	// Wait until the query starts running.
	vtesting.WaitUntil(time.Second*50, self.T(), func() bool {
		return len(actions.QueryLog.Get()) > 0
	})

	// The cancel message should generate 1 log + a status
	// message. This should only be done once, no matter how many
	// cancellations are sent.
	vtesting.WaitUntil(time.Second*5, self.T(), func() bool {
		received_messages := collector.Messages()

		// Should be at least one stat message and several log messages
		return len(received_messages) >= 3 &&
			getFlowStat(received_messages) != nil
	})

	received_messages := collector.Messages()

	// An error status
	stats := getFlowStat(received_messages)
	assert.Equal(self.T(), crypto_proto.VeloStatus_GENERIC_ERROR,
		stats.FlowStats.QueryStatus[0].Status)

	assert.Contains(self.T(), stats.FlowStats.QueryStatus[0].ErrorMessage,
		"Upload bytes 37 exceeded limit 10 for flow")

	assert.Contains(self.T(), getLogMessages(received_messages),
		"Cancelled all inflight queries")
}

// Exceeding row limit will cancel flow
func (self *ExecutorTestSuite) TestRowLimitCancellation() {
	t := self.T()

	// Max time for deadlock detection.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()

	config_obj := config.GetDefaultConfig()
	executor, err := NewClientExecutor(ctx, "", config_obj)
	require.NoError(t, err)

	flow_id := "F.TestRowLimitCancellation"

	actions.QueryLog.Clear()

	collector := NewMessageCollector(ctx, executor)

	// Send the executor a flow request
	executor.Inbound <- &crypto_proto.VeloMessage{
		AuthState: crypto_proto.VeloMessage_AUTHENTICATED,
		SessionId: flow_id,
		FlowRequest: &crypto_proto.FlowRequest{
			// Only 10 bytes are allowed.
			MaxRows: 10,
			VQLClientActions: []*actions_proto.VQLCollectorArgs{{
				// Do not batch rows before sending them. The
				// FlowContext only does limit checks on completed row
				// batches (normally 1000 rows at a time).
				MaxRow: 1,
				Query: []*actions_proto.VQLRequest{
					{
						Name: "Query",
						VQL:  `SELECT _value FROM range(end=100)`,
					},
				},
			}},
		},
	}

	// Wait until the query starts running.
	vtesting.WaitUntil(time.Second*50, self.T(), func() bool {
		return len(actions.QueryLog.Get()) > 0
	})

	// The cancel message should generate 1 log + a status
	// message. This should only be done once, no matter how many
	// cancellations are sent.
	vtesting.WaitUntil(time.Second*5, self.T(), func() bool {
		received_messages := collector.Messages()

		// Should be at least one stat message and several log messages
		return len(received_messages) >= 3 &&
			getFlowStat(received_messages) != nil
	})

	received_messages := collector.Messages()

	// An error status
	stats := getFlowStat(received_messages)

	assert.Equal(self.T(), crypto_proto.VeloStatus_GENERIC_ERROR,
		stats.FlowStats.QueryStatus[0].Status)
	assert.Contains(self.T(), stats.FlowStats.QueryStatus[0].ErrorMessage,
		"Rows 11 exceeded limit 10 for flow")
	assert.Contains(self.T(), getLogMessages(received_messages),
		"Cancelled all inflight queries")
}

// Test the total result row count is accurate
func (self *ExecutorTestSuite) TestTotalRowCount() {
	t := self.T()

	// Max time for deadlock detection.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()

	config_obj := config.GetDefaultConfig()
	executor, err := NewClientExecutor(ctx, "", config_obj)
	require.NoError(t, err)

	flow_id := "F.TestRowCount"

	actions.QueryLog.Clear()

	collector := NewMessageCollector(ctx, executor)

	// Send the executor a flow request with two SELECT statements
	// (for two sources)
	executor.Inbound <- &crypto_proto.VeloMessage{
		AuthState: crypto_proto.VeloMessage_AUTHENTICATED,
		SessionId: flow_id,
		FlowRequest: &crypto_proto.FlowRequest{
			// Only 10 bytes are allowed.
			MaxRows: 10,
			VQLClientActions: []*actions_proto.VQLCollectorArgs{{
				Query: []*actions_proto.VQLRequest{{
					Name: "Query",
					VQL:  `SELECT 1 FROM scope()`,
				}, {
					Name: "Query2",
					VQL:  `SELECT 2 FROM scope()`,
				}},
			}},
		},
	}

	// The cancel message should generate 1 log + a status
	// message. This should only be done once, no matter how many
	// cancellations are sent.
	vtesting.WaitUntil(time.Second*5, self.T(), func() bool {
		return getFlowStat(collector.Messages()) != nil
	})

	// An error status
	stats := getFlowStat(collector.Messages())

	assert.Equal(self.T(), crypto_proto.VeloStatus_OK,
		stats.FlowStats.QueryStatus[0].Status)
	assert.Equal(self.T(), int64(2),
		stats.FlowStats.QueryStatus[0].ResultRows)
}

func getFlowStat(messages []*crypto_proto.VeloMessage) *crypto_proto.VeloMessage {
	for _, m := range messages {
		if m.FlowStats != nil {
			return m
		}
	}
	return nil
}

func getLogMessages(messages []*crypto_proto.VeloMessage) string {
	res := ""
	for _, m := range messages {
		if m.LogMessage != nil {
			res += m.LogMessage.Jsonl
		}
	}
	return res
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

	collector := NewMessageCollector(ctx, executor)

	// Send two requests for the same flow in parallel these should
	// generate a bunch of log messages.
	flow_id := fmt.Sprintf("F.XXX%d", utils.GetId())
	executor.Inbound <- &crypto_proto.VeloMessage{
		AuthState: crypto_proto.VeloMessage_AUTHENTICATED,
		SessionId: flow_id,
		FlowRequest: &crypto_proto.FlowRequest{
			VQLClientActions: []*actions_proto.VQLCollectorArgs{{
				Query: []*actions_proto.VQLRequest{
					// Log 10 messages
					{
						Name: "LoggingArtifact",
						VQL:  "SELECT log(message='log %v', args=count(), dedup= -1) FROM range(end=10)",
					},
				}}},
		},
		RequestId: 1}

	// collect the log messages and ensure they are all batched in one response.
	log_messages := []*crypto_proto.LogMessage{}

	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		var total_messages uint64
		log_messages = nil

		for _, msg := range collector.Messages() {
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

// Test the error messages are generated when logs match.
func (self *ExecutorTestSuite) TestErrorRegexLogMessages() {
	t := self.T()

	// Max time for deadlock detection.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	config_obj := config.GetDefaultConfig()
	executor, err := NewClientExecutor(ctx, "", config_obj)
	require.NoError(t, err)

	collector := NewMessageCollector(ctx, executor)

	// Send two requests for the same flow in parallel these should
	// generate a bunch of log messages.
	flow_id := fmt.Sprintf("F.XXX%d", utils.GetId())
	executor.Inbound <- &crypto_proto.VeloMessage{
		AuthState: crypto_proto.VeloMessage_AUTHENTICATED,
		SessionId: flow_id,
		FlowRequest: &crypto_proto.FlowRequest{
			// Specify a custom error regex which should trigger.
			LogErrorRegex: "i am an error",
			VQLClientActions: []*actions_proto.VQLCollectorArgs{{
				Query: []*actions_proto.VQLRequest{
					// Log 10 messages
					{
						Name: "LoggingArtifact",
						VQL:  "SELECT log(message='i am an error message') FROM scope()",
					},
				}}},
		},
		RequestId: 1}

	// collect the log messages and ensure they are all batched in one response.
	messages := []*crypto_proto.VeloMessage{}

	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		messages = nil

		for _, msg := range collector.Messages() {
			if msg.FlowStats != nil && msg.FlowStats.FlowComplete {
				messages = append(messages, msg)
			}
		}

		return len(messages) > 0
	})

	// This should be marked as an error.
	error_message := messages[0].FlowStats.QueryStatus[0].ErrorMessage
	assert.Contains(self.T(), error_message, "i am an error message")
}

// Schedule a flow in the database and return its flow id
func (self ExecutorTestSuite) createArtifactCollection(name string) (string, error) {
	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(self.ConfigObj)
	require.NoError(self.T(), err)

	// Schedule a flow in the database.
	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	flow_id, err := launcher.ScheduleArtifactCollection(self.Ctx,
		self.ConfigObj,
		acl_managers.NullACLManager{},
		repository,
		&flows_proto.ArtifactCollectorArgs{
			ClientId:  self.client_id,
			Creator:   utils.GetSuperuserName(self.ConfigObj),
			Artifacts: []string{name},
		}, nil)

	return flow_id, err
}

func (self *ExecutorTestSuite) TestErrorMessage() {
	t := self.T()

	// Max time for deadlock detection.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	config_obj := config.GetDefaultConfig()
	executor, err := NewClientExecutor(ctx, self.client_id, config_obj)
	require.NoError(t, err)

	collector := NewMessageCollector(ctx, executor)

	// Send two requests for the same flow in parallel these should
	// generate a bunch of log messages.
	flow_id, err := self.createArtifactCollection("Generic.Client.Info")
	assert.NoError(self.T(), err)

	executor.Inbound <- &crypto_proto.VeloMessage{
		AuthState: crypto_proto.VeloMessage_AUTHENTICATED,
		SessionId: flow_id,
		FlowRequest: &crypto_proto.FlowRequest{
			LogBatchTime: 1,
			VQLClientActions: []*actions_proto.VQLCollectorArgs{{
				MaxWait: 1,
				Query: []*actions_proto.VQLRequest{
					{
						Name: "Generic.Client.Info",
						VQL: `
SELECT sleep(ms=400) AS Sleep, now() AS Now FROM range(end=4)
WHERE log(message="this is an error", level="ERROR")`,
					},
				}}},
		},
		RequestId: 1}

	vtesting.WaitUntil(10*time.Second, self.T(), func() bool {
		msgs := collector.Messages()
		if len(msgs) == 0 {
			return false
		}

		for _, m := range msgs {
			if m.FlowStats != nil && m.FlowStats.FlowComplete {
				return true
			}
		}
		return false
	})

	// Now process these messages
	// Emulate a response from this flow.
	runner := flows.NewFlowRunner(self.Ctx, self.ConfigObj)
	for _, msg := range collector.Messages() {
		msg.Source = self.client_id
		runner.ProcessSingleMessage(self.Ctx, msg)
	}
	runner.Close(self.Ctx)

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	details, err := launcher.GetFlowDetails(self.Ctx, self.ConfigObj,
		services.GetFlowOptions{}, self.client_id, flow_id)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), "this is an error\n", details.Context.Status)
	assert.Equal(self.T(), details.Context.QueryStats[0].NamesWithResponse, []string{
		"Generic.Client.Info",
	})
	assert.Equal(self.T(), details.Context.QueryStats[0].ResultRows, int64(4))
}

// Check that the executor returns the correct status for running flows.
func (self *ExecutorTestSuite) TestFlowStatsRequest() {
	t := self.T()

	closer := utils.MockTime(utils.NewMockClock(time.Unix(100, 0)))
	defer closer()

	// Max time for deadlock detection.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()

	config_obj := config.GetDefaultConfig()
	executor, err := NewClientExecutor(ctx, "", config_obj)
	require.NoError(t, err)

	flow_id := "F.XXX_TestFlowStatsRequest"

	actions.QueryLog.Clear()

	collector := NewMessageCollector(ctx, executor)

	// Send the executor a flow request
	executor.Inbound <- &crypto_proto.VeloMessage{
		AuthState: crypto_proto.VeloMessage_AUTHENTICATED,
		SessionId: flow_id,
		FlowRequest: &crypto_proto.FlowRequest{
			VQLClientActions: []*actions_proto.VQLCollectorArgs{{
				Query: []*actions_proto.VQLRequest{
					{
						Name: "Query",
						VQL:  "SELECT sleep(time=1000) FROM scope()",
					},
				},
			}},
		},
	}

	// Wait until the query starts running.
	vtesting.WaitUntil(time.Second*50, self.T(), func() bool {
		return len(actions.QueryLog.Get()) > 0
	})

	// Send the executor a flow request
	executor.Inbound <- &crypto_proto.VeloMessage{
		AuthState: crypto_proto.VeloMessage_AUTHENTICATED,
		SessionId: constants.STATUS_CHECK_WELL_KNOWN_FLOW,
		FlowStatsRequest: &crypto_proto.FlowStatsRequest{
			FlowId: []string{flow_id, "F.NotExists"},
		},
	}

	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		// Should be two messages - first is status of the real flow,
		// second is a "Flow not known - maybe the client crashed?"
		// message for the not existant flow we dont know about. This
		// should cause the server to error out that outstanding flow.
		return len(collector.Messages()) == 2
	})

	goldie.Assert(self.T(), "TestFlowStatsRequest",
		json.MustMarshalIndent(collector.Messages()))
}

func TestExecutorTestSuite(t *testing.T) {
	suite.Run(t, &ExecutorTestSuite{
		client_id: "C.1234",
	})
}
