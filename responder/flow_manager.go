/*
  Keep track of queries and flows:

1. A FlowManager is a global service that runs on the client. The
   FlowManager keeps track of currently running flows.

2. FlowManager.FlowContext() creates a new flow context to manage this
   session id, or retrieves an existing FlowContext.

3. The FlowContext manages the stats of a flow on the client. A Flow
   may contain several queries as well as Stats (total rows, total
   files uploaded etc).

4. A QueryContext is issues for each query within the flow
   context. The QueryContext contains the cancellable context for the
   currently running query.

*/

package responder

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	mu                  sync.Mutex
	_FlowManagerService *FlowManager
)

// QueryContext keeps track of a single query request running as part
// of a given flow. There can be multiple Queries for each Flow, and
// therefore we keep multiple QueryContext instances for each query.
// When the flow is cancelled we cancel all the related query contexts
// at once.
type QueryContext struct {
	cancel  func()
	id      uint64
	flow_id string
	req     *crypto_proto.VeloMessage
}

// Represents a single flow on the client. Previously flows were
// tracked on the server but now they are completely tacked on the
// client, and simply synced to the server. This dramatically reduces
// the amount of work done on the server.
type FlowContext struct {
	ctx    context.Context
	cancel func()

	flow_id string

	// Query contexts that make up the flow.
	queries map[uint64]*QueryContext

	// A counter of uploads sent in the entire collection.
	upload_id int32

	// A JSONL buffer with log messages collected for the entire flow.
	mu                sync.Mutex
	log_messages      []byte
	log_messages_id   uint64 // The ID of the first row in the log_messages buffer
	log_message_count uint64
	error_message     string // If an error occurs trap the error message

	// Keep stats of the flow.
	Stats Stats
}

func NewFlowContext(ctx context.Context, flow_id string) *FlowContext {
	frequency_sec := uint64(5)

	sub_ctx, cancel := context.WithCancel(ctx)

	return &FlowContext{
		ctx:     sub_ctx,
		cancel:  cancel,
		flow_id: flow_id,
		queries: make(map[uint64]*QueryContext),
		Stats: Stats{
			FlowStats:     &crypto_proto.FlowStats{},
			frequency_sec: frequency_sec,
		},
	}
}

// Drains the error message buffer for transmission
func (self *FlowContext) GetLogMessages() (
	buf []byte, start_id uint64, message_count uint64, error_message string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	buf = self.log_messages
	message_count = self.log_message_count
	start_id = self.log_messages_id
	error_message = self.error_message

	self.log_messages = nil
	self.log_message_count = 0
	self.log_messages_id = start_id + message_count
	self.error_message = ""

	return buf, start_id, message_count, error_message
}

func (self *FlowContext) AddLogMessage(level string, msg, artifact string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Capture the first message at error level. This allows the
	// server to skip parsing the jsonl bundle completely.
	if level == logging.ERROR && self.error_message == "" {
		self.error_message = msg
	}

	self.log_message_count++
	self.log_messages = append(self.log_messages, json.Format(
		"{\"client_time\":%d,\"level\":%q,\"message\":%q}\n",
		int(time.Now().Unix()), level, msg)...)
}

func (self *FlowContext) NextUploadId() int64 {
	new_id := int64(atomic.AddInt32(&self.upload_id, 1))
	return new_id - 1
}

func (self *FlowContext) NewQueryContext(
	responder *Responder, req *crypto_proto.VeloMessage) (
	ctx context.Context, closer func()) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Cancellable context for the query
	ctx, cancel := context.WithCancel(self.ctx)

	result := &QueryContext{
		flow_id: self.flow_id,
		cancel:  cancel,
		req:     req,
		id:      utils.GetId(),
	}

	self.queries[result.id] = result

	return ctx, func() {
		cancel()

		self.mu.Lock()
		delete(self.queries, result.id)
		remaining_queries := len(self.queries)
		self.mu.Unlock()

		// When there are no more outstanding queries, send the final
		// response.
		if remaining_queries == 0 {
			// Flush any waiting logs now
			buf, id, count, error_message := self.GetLogMessages()
			if len(buf) > 0 {
				fmt.Printf("Sending %v log messages\n", count)
				responder.AddResponse(&crypto_proto.VeloMessage{
					RequestId: constants.LOG_SINK,
					LogMessage: &crypto_proto.LogMessage{
						Id:           int64(id),
						NumberOfRows: count,
						Jsonl:        string(buf),
						ErrorMessage: error_message,
					}})
			}

			// Send the stats one last time.
			stats := self.Stats.GetStats()
			stats.FlowComplete = true

			if stats != nil {
				responder.AddResponse(&crypto_proto.VeloMessage{
					RequestId: constants.STATS_SINK,
					FlowStats: stats,
				})
			}
		}
	}
}

// A Flow Manager runs on the client and keeps track of all flows that
// are running on the client. The server may request flows to be
// cancelled at any time, which allows the manager to cancel in flight
// queries that belong to the same flow.
type FlowManager struct {
	mu sync.Mutex

	ctx        context.Context
	config_obj *config_proto.Config
	in_flight  map[string]*FlowContext
	next_id    int

	cancelled map[string]bool
}

func NewFlowManager(
	ctx context.Context, config_obj *config_proto.Config) *FlowManager {

	if utils.IsNil(ctx) {
		panic(ctx)
	}

	return &FlowManager{
		ctx:        ctx,
		config_obj: config_obj,
		in_flight:  make(map[string]*FlowContext),
		cancelled:  make(map[string]bool),
	}
}

func (self *FlowManager) IsCancelled(flow_id string) bool {
	self.mu.Lock()
	defer self.mu.Unlock()
	ok, _ := self.cancelled[flow_id]
	return ok
}

func (self *FlowManager) Cancel(ctx context.Context, flow_id string) {

	// Some flows are non-cancellable.
	switch flow_id {
	case constants.MONITORING_WELL_KNOWN_FLOW:
		return
	}

	self.mu.Lock()
	ok, _ := self.cancelled[flow_id]
	if ok {
		self.mu.Unlock()
		return
	}

	self.cancelled[flow_id] = true

	flow_context, ok := self.in_flight[flow_id]
	if !ok {
		self.mu.Unlock()
		return
	}

	delete(self.in_flight, flow_id)
	self.mu.Unlock()

	flow_context.cancel()
}

func (self *FlowManager) FlowContext(
	request *crypto_proto.VeloMessage) *FlowContext {

	flow_id := request.SessionId

	self.mu.Lock()
	defer self.mu.Unlock()

	flow_context, ok := self.in_flight[flow_id]
	if !ok {
		flow_context = NewFlowContext(self.ctx, flow_id)
		self.in_flight[flow_id] = flow_context
	}

	return flow_context
}

func GetFlowManager(
	ctx context.Context, config_obj *config_proto.Config) *FlowManager {
	mu.Lock()
	defer mu.Unlock()

	if _FlowManagerService == nil {
		_FlowManagerService = NewFlowManager(ctx, config_obj)
	}

	return _FlowManagerService
}
