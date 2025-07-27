package executor

import (
	"context"

	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *ClientExecutor) ResumeTransactions(
	ctx context.Context,
	config_obj *config_proto.Config, req *crypto_proto.VeloMessage) {

	if req.ResumeTransactions == nil {
		return
	}

	// Uncancel the flow.
	self.flow_manager.UnCancel(req.SessionId)

	flow_context := self.flow_manager.FlowContext(self.Outbound, req)
	defer flow_context.Close()

	// Responses for transactions go into a special result set called
	// "Resumed Uploads". If the flow was resumed previously the query
	// stats already have such an artifact, otherwise we create a new
	// one.
	var our_responder responder.Responder
	var our_stat *crypto_proto.VeloStatus

	for _, stat := range req.ResumeTransactions.QueryStats {
		_, responder_obj := flow_context.NewResponder(
			&actions_proto.VQLCollectorArgs{})
		defer responder_obj.Close()

		responder_obj.SetStatus(stat)

		if utils.InString(stat.NamesWithResponse,
			constants.UPLOAD_RESUMED_SOURCE) {
			our_responder = responder_obj
			our_stat = stat
		}
	}

	if our_responder == nil {
		_, new_responder := flow_context.NewResponder(
			&actions_proto.VQLCollectorArgs{})
		new_responder.SetStatus(&crypto_proto.VeloStatus{
			Status:            crypto_proto.VeloStatus_PROGRESS,
			NamesWithResponse: []string{constants.UPLOAD_RESUMED_SOURCE},
		})

		our_responder = new_responder
		our_stat = &crypto_proto.VeloStatus{}
	}

	actions.ResumeTransactions(
		ctx, config_obj, our_responder, our_stat, req.ResumeTransactions)
}
