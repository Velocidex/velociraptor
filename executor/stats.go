package executor

import (
	"context"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

// This request asks the client to report progress on all currently
// running flows. This is needed for the server to identify when a
// client has restarted or crashed and reap any flows that are still
// outstanding.
func (self *ClientExecutor) ProcessStatRequest(
	ctx context.Context,
	config_obj *config_proto.Config, req *crypto_proto.VeloMessage) {

	for _, flow_id := range req.FlowStatsRequest.FlowId {
		flow_context, err := self.flow_manager.Get(flow_id)

		var stats *crypto_proto.VeloMessage
		if utils.IsNotFound(err) {
			stats = &crypto_proto.VeloMessage{
				SessionId: flow_id,
				RequestId: constants.STATS_SINK,
				FlowStats: &crypto_proto.FlowStats{
					FlowComplete: true,
					QueryStatus: []*crypto_proto.VeloStatus{{
						Status:       crypto_proto.VeloStatus_UNKNOWN_FLOW,
						ErrorMessage: "Flow not known - maybe the client crashed?",
					}},
				},
			}
		} else {
			stats = flow_context.GetStats()
		}

		// Make sure responses get there immediately.
		stats.Urgent = true
		self.SendToServer(stats)
	}
}
