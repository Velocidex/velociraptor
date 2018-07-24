// The foreman is a Well Known Flow for clients to check for hunt
// memberships. Each client periodically informs the foreman on the
// most recent hunt it executed, and the foreman launches the relevant
// flow on the client.

// The process goes like this:

// 1. The client sends a message to the foreman periodically with the
//    timestamp of the most recent hunt it ran.
// 2. The foreman then starts a conditional flow for each client. The
//    conditional flow runs a VQL query on the client and determines
//    if another flow should be run based on a codition posed on the
//    VQL response.
// 3. If the condition is satisfied, the flow proceeds to run

package flows

import (
	"errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config "www.velocidex.com/golang/velociraptor/config"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/responder"
	//	utils "www.velocidex.com/golang/velociraptor/testing"
)

type Foreman struct {
	BaseFlow
}

func (self *Foreman) ProcessMessage(
	config_obj *config.Config,
	flow_obj *AFF4FlowObject,
	message *crypto_proto.GrrMessage) error {
	foreman_checkin, ok := responder.ExtractGrrMessagePayload(
		message).(*actions_proto.ForemanCheckin)
	if !ok {
		return errors.New("Expected args of type ForemanCheckin")
	}

	hunts_dispatcher, err := GetHuntDispatcher(config_obj)
	if err != nil {
		return err
	}

	for _, hunt := range hunts_dispatcher.GetApplicableHunts(
		foreman_checkin.LastHuntTimestamp) {

		// Start a conditional flow.
		flow_runner_args := &flows_proto.FlowRunnerArgs{
			ClientId: message.Source,
			FlowName: "CheckHuntCondition",
		}

		err := SetFlowArgs(flow_runner_args, hunt)
		if err != nil {
			return err
		}

		flow_id, err := StartFlow(config_obj, flow_runner_args)
		if err != nil {
			return err
		}

		_ = flow_id
	}
	return nil
}
