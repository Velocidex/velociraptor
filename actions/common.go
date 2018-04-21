package actions

import (
	"www.velocidex.com/golang/velociraptor/context"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

type GetClientInfo struct{}

func (self *GetClientInfo) Run(
	ctx *context.Context,
	args *crypto_proto.GrrMessage) []*crypto_proto.GrrMessage {
	responder := NewResponder(args)
	info := &actions_proto.ClientInformation{
		ClientName: &ctx.Config.Client_name,
		ClientVersion: &ctx.Config.Client_version,
		Labels: ctx.Config.Client_labels,
	}
	responder.AddResponse(info)
	return responder.Return()
}
