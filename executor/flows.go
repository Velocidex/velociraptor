package executor

import (
	"context"

	"www.velocidex.com/golang/velociraptor/actions"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/responder"
)

// New servers issue a FlowRequest message forcing the client to
// process and track the entire collection at once.
func (self *ClientExecutor) ProcessFlowRequest(
	ctx context.Context,
	config_obj *config_proto.Config, req *crypto_proto.VeloMessage) {

	flow_manager := responder.GetFlowManager(ctx, config_obj)
	flow_context := flow_manager.FlowContext(self.Outbound, req)
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

	for _, arg := range req.FlowRequest.VQLClientActions {
		// A responder is used to track each specific query within the
		// entire flow. There can be multiple queries run in parallel
		// within the same flow.
		sub_ctx, responder_obj := flow_context.NewResponder(arg)
		defer responder_obj.Close()

		actions.VQLClientAction{}.StartQuery(
			config_obj, sub_ctx, responder_obj, arg)
	}
}
