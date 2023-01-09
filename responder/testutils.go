package responder

import (
	"context"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

func TestResponder(config_obj *config_proto.Config) *Responder {
	ctx, cancel := context.WithCancel(context.Background())
	flow_manager := GetFlowManager(ctx, config_obj)
	result := &Responder{
		ctx:     ctx,
		cancel:  cancel,
		output:  make(chan *crypto_proto.VeloMessage, 100),
		request: &crypto_proto.VeloMessage{SessionId: "F.Test"},
	}

	result.flow_context = flow_manager.FlowContext(result.request)
	return result
}

func GetTestResponses(self *Responder) []*crypto_proto.VeloMessage {
	close(self.output)
	result := []*crypto_proto.VeloMessage{}
	for item := range self.output {
		result = append(result, item)
	}

	return result
}
