/*
  Keep track of queries and flows:

1. A FlowManager is a global service that runs on the client. The
   FlowManager keeps track of currently running flows.

2. FlowManager.FlowContext() creates a new flow context to manage this
   session id, or retrieves an existing FlowContext.

3. The FlowContext manages the stats of a flow on the client. A
   FlowContext may contain several QueryContext as well as Stats
   (total rows, total files uploaded etc).

4. A QueryContext is issued for each query within the flow
   context. The QueryContext contains the cancellable context for the
   currently running query.

*/

package responder

import (
	"context"
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

// A Flow Manager runs on the client and keeps track of all flows that
// are running on the client. The server may request flows to be
// cancelled at any time, which allows the manager to cancel in flight
// queries that belong to the same flow.
type FlowManager struct {
	mu sync.Mutex

	id uint64

	ctx        context.Context
	config_obj *config_proto.Config
	in_flight  map[string]*FlowContext
	next_id    int

	// Remember all the cancelled sessions so the ring buffer file can
	// drop any messages for flows that were already cancelled.
	cancelled map[string]bool
}

func NewFlowManager(ctx context.Context,
	config_obj *config_proto.Config) *FlowManager {

	result := &FlowManager{
		ctx:        ctx,
		id:         utils.GetId(),
		config_obj: config_obj,
		in_flight:  make(map[string]*FlowContext),
		cancelled:  make(map[string]bool),
	}
	return result
}

func (self *FlowManager) removeFlowContext(flow_id string) {
	self.mu.Lock()
	delete(self.in_flight, flow_id)
	self.mu.Unlock()
}

func (self *FlowManager) IsCancelled(flow_id string) bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	_, pres := self.cancelled[flow_id]
	return pres
}

func (self *FlowManager) Cancel(ctx context.Context, flow_id string) {

	// Some flows are non-cancellable.
	switch flow_id {
	case constants.MONITORING_WELL_KNOWN_FLOW:
		return
	}

	self.mu.Lock()
	// Remember that this flow is already cancelled - this will be
	// used to remove useless messages from the ring buffer file.
	self.cancelled[flow_id] = true

	// Do we know about this flow? If the flow is already done we
	// ignore the cancel request.
	flow_context, pres := self.in_flight[flow_id]
	self.mu.Unlock()
	if pres {
		flow_context.Cancel()
	}
}

func (self *FlowManager) FlowContext(
	output chan *crypto_proto.VeloMessage,
	req *crypto_proto.VeloMessage) *FlowContext {

	flow_id := req.SessionId

	self.mu.Lock()
	defer self.mu.Unlock()

	flow_context, pres := self.in_flight[flow_id]
	if !pres {
		flow_context := newFlowContext(
			self.ctx, self.config_obj, output, req, self)

		self.in_flight[flow_id] = flow_context
		return flow_context
	}
	return flow_context
}
