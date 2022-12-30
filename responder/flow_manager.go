package responder

import (
	"context"
	"sync"
	"sync/atomic"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
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
	id      int
	flow_id string
}

// Represents a single flow on the client
type FlowContext struct {
	// A list of query contexts that make up the flow.
	queries []*QueryContext

	log_id int32
}

func (self *FlowContext) NextLogId() int64 {
	return int64(atomic.AddInt32(&self.log_id, 1))
}

func (self *FlowContext) Queries() []*QueryContext {
	return self.queries
}

func (self *FlowContext) AddQuery(ctx *QueryContext) {
	self.queries = append(self.queries, ctx)
}

func (self *FlowContext) RemoveQuery(ctx *QueryContext) {
	new_context := make([]*QueryContext, 0, len(self.queries))
	for _, q := range self.queries {
		if q.id != ctx.id {
			new_context = append(new_context, q)
		}
	}

	self.queries = new_context
}

// A Flow Manager runs on the client and keeps track of all flows that
// are running on the client. The server may request flows to be
// cancelled at any time, which allows the manager to cancel in flight
// queries that belong to the same flow.
type FlowManager struct {
	mu sync.Mutex

	config_obj *config_proto.Config
	in_flight  map[string]*FlowContext
	next_id    int

	cancelled map[string]bool
}

func NewFlowManager(config_obj *config_proto.Config) *FlowManager {
	return &FlowManager{
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

	// Cancel all existing queries.
	for _, query_ctx := range flow_context.Queries() {
		query_ctx.cancel()
	}
}

// Track a flow and return a new context. The context can be used to
// start the query which can be cancelled if the flow is cancelled.
func (self *FlowManager) NewQueryContext(flow_id string) (
	ctx context.Context, closer func()) {
	self.mu.Lock()
	defer self.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())

	result := &QueryContext{
		flow_id: flow_id,
		cancel:  cancel,
		id:      self.next_id,
	}
	self.next_id++

	flow_context, ok := self.in_flight[flow_id]
	if !ok {
		flow_context = &FlowContext{}
	}
	flow_context.AddQuery(result)
	self.in_flight[flow_id] = flow_context

	return ctx, func() { self.closeContext(result) }
}

func (self *FlowManager) FlowContext(flow_id string) *FlowContext {
	self.mu.Lock()
	defer self.mu.Unlock()

	flow_context, ok := self.in_flight[flow_id]
	if !ok {
		flow_context = &FlowContext{}
		self.in_flight[flow_id] = flow_context
	}

	return flow_context
}

// _CloseContext removes the flow_context from the in_flight map.
// Note: There are multiple queries tied to the same flow id but all
// of them need to be cancelled when the flow is cancelled.
func (self *FlowManager) closeContext(query_context *QueryContext) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Scan through all related query contexts (for the same flow) and
	// remove this specific one.
	flow_id := query_context.flow_id

	flow_context, ok := self.in_flight[flow_id]
	if ok {
		flow_context.RemoveQuery(query_context)

		if len(flow_context.Queries()) == 0 {
			delete(self.in_flight, flow_id)
		}
	}
}

func GetFlowManager(config_obj *config_proto.Config) *FlowManager {
	mu.Lock()
	defer mu.Unlock()

	if _FlowManagerService == nil {
		_FlowManagerService = NewFlowManager(config_obj)
	}

	return _FlowManagerService
}
