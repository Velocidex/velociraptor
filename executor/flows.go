package executor

import (
	"context"
	"sync"

	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/responder"
)

// New servers issue a FlowRequest message forcing the client to
// process and track the entire collection at once.
func (self *ClientExecutor) ProcessFlowRequest(
	ctx context.Context,
	config_obj *config_proto.Config, req *crypto_proto.VeloMessage) {

	flow_context := self.flow_manager.FlowContext(self.Outbound, req)
	defer flow_context.Close()

	// Control concurrency for the entire collection at once. If a
	// collection has many queries, they all run concurrently.
	if !req.Urgent {
		cancel, err := self.concurrency.StartConcurrencyControl(ctx)
		if err != nil {
			responder.MakeErrorResponse(
				self.Outbound, req.SessionId, err.Error())
			return
		}
		defer cancel()
	}

	// Wait for the trace to finish recording all its data.
	trace_wg := &sync.WaitGroup{}
	defer trace_wg.Wait()

	// Cancel traces when the entire collection exist.
	trace_ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Run trace queries now but do not wait for them to exit before
	// cancelling them.
	for _, arg := range req.FlowRequest.Trace {
		trace_wg.Add(1)
		go func(arg *actions_proto.VQLCollectorArgs) {
			defer trace_wg.Done()

			_, responder_obj := flow_context.NewResponder(arg)
			defer responder_obj.Close()

			actions.VQLClientAction{}.StartQuery(
				config_obj, trace_ctx, responder_obj, arg)
		}(arg)

	}

	// Wait for all subqueries before closing the collection.
	wg := &sync.WaitGroup{}
	defer wg.Wait()

	for _, arg := range req.FlowRequest.VQLClientActions {
		wg.Add(1)

		// Run each VQLClientActions in another goroutine.
		go func(arg *actions_proto.VQLCollectorArgs) {
			defer wg.Done()

			// A responder is used to track each specific query within the
			// entire flow. There can be multiple queries run in parallel
			// within the same flow.
			sub_ctx, responder_obj := flow_context.NewResponder(arg)
			defer responder_obj.Close()

			actions.VQLClientAction{}.StartQuery(
				config_obj, sub_ctx, responder_obj, arg)
		}(arg)
	}
}
