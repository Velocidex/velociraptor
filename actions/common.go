package actions

import (
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/context"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

type GetClientInfo struct{}

func (self *GetClientInfo) Run(
	ctx *context.Context,
	args *crypto_proto.GrrMessage,
	output chan<- *crypto_proto.GrrMessage) {
	responder := NewResponder(args, output)
	info := &actions_proto.ClientInformation{
		ClientName:    &ctx.Config.Client_name,
		ClientVersion: &ctx.Config.Client_version,
		Labels:        ctx.Config.Client_labels,
	}
	responder.AddResponse(info)
	responder.Return()
}
