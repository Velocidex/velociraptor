package executor

import (
	"context"
	"fmt"

	"www.velocidex.com/golang/velociraptor/actions"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/responder"
)

func (self *ClientExecutor) ProcessFlowRequest(
	ctx context.Context,
	config_obj *config_proto.Config, req *crypto_proto.VeloMessage) {

	flow_manager := responder.GetFlowManager(ctx, config_obj)
	flow_context := flow_manager.FlowContext(req)

	// Control concurrency for the entire collection at once.
	if !req.Urgent {
		cancel, err := self.concurrency.StartConcurrencyControl(ctx)
		if err != nil {
			responder_obj := responder.NewResponder(ctx, config_obj,
				req, self.Outbound)
			defer responder_obj.Close()

			responder_obj.RaiseError(ctx, fmt.Sprintf("%v", err))
			return
		}
		defer cancel()
	}

	for _, arg := range req.FlowRequest.VQLClientActions {
		responder_obj := responder.NewResponder(ctx, config_obj,
			req, self.Outbound)
		defer responder_obj.Close()

		// Each request has its own context.
		query_ctx, closer := flow_context.NewQueryContext(responder_obj)
		defer closer()

		actions.VQLClientAction{}.StartQuery(
			config_obj, query_ctx, responder_obj, arg)
	}
}
