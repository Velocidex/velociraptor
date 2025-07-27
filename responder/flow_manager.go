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
	"fmt"
	"sync"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/velociraptor/third_party/cache"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
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

	// These are old flows that are completed.
	finished *cache.LRUCache

	// Remember all the cancelled sessions so the ring buffer file can
	// drop any messages for flows that were already cancelled.
	cancelled map[string]bool
}

func NewFlowManager(ctx context.Context,
	config_obj *config_proto.Config, client_id string) *FlowManager {

	result := &FlowManager{
		ctx:        ctx,
		id:         utils.GetId(),
		config_obj: config_obj,
		in_flight:  make(map[string]*FlowContext),
		finished:   cache.NewLRUCache(20),
		cancelled:  make(map[string]bool),
	}

	debug.RegisterProfileWriter(debug.ProfileWriterInfo{
		Name: "ClientFlowManager",
		Description: fmt.Sprintf(
			"Report the state of the client's flow manager (%v)", client_id),
		ProfileWriter: result.WriteProfile,
		Categories:    []string{"Client"},
	})

	return result
}

func (self *FlowManager) WriteProfile(ctx context.Context,
	scope vfilter.Scope, output_chan chan vfilter.Row) {

	var results []*ordereddict.Dict

	self.mu.Lock()
	for flow_id, flow_context := range self.in_flight {
		results = append(results, ordereddict.NewDict().
			Set("FlowId", flow_id).
			Set("State", "In Flight").
			Set("Stats", flow_context.GetStatsDicts()))
	}

	for _, flow_id := range self.finished.Keys() {
		flow_context_any, pres := self.finished.Peek(flow_id)
		if !pres {
			continue
		}

		flow_context, ok := flow_context_any.(*FlowContext)
		if !ok {
			continue
		}

		results = append(results, ordereddict.NewDict().
			Set("FlowId", flow_id).
			Set("State", "Completed").
			Set("Stats", flow_context.GetStatsDicts()))
	}

	for flow_id := range self.cancelled {
		results = append(results, ordereddict.NewDict().
			Set("FlowId", flow_id).
			Set("State", "Cancelled"))
	}
	defer self.mu.Unlock()

	for _, r := range results {
		select {
		case <-ctx.Done():
			return
		case output_chan <- r:
		}
	}
}

func (self *FlowManager) RemoveFlowContext(flow_id string) {
	self.mu.Lock()

	flow_context, pres := self.in_flight[flow_id]
	if !pres {
		self.mu.Unlock()
		return
	}

	delete(self.in_flight, flow_id)

	self.mu.Unlock()

	// Append the flow to the completed list.
	self.finished.Set(flow_id, flow_context)
}

func (self *FlowManager) IsCancelled(flow_id string) bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	_, pres := self.cancelled[flow_id]
	return pres
}

func (self *FlowManager) UnCancel(flow_id string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	delete(self.cancelled, flow_id)
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

func (self *FlowManager) Get(flow_id string) (*FlowContext, error) {
	self.mu.Lock()
	flow_context, pres := self.in_flight[flow_id]
	self.mu.Unlock()

	if !pres {
		// Flow is not in flight - maybe it is finished?
		flow_context_any, pres := self.finished.Get(flow_id)
		if pres {
			flow_context, ok := flow_context_any.(*FlowContext)
			if ok {
				return flow_context, nil
			}
		}
		return nil, utils.NotFoundError
	}

	return flow_context, nil
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
