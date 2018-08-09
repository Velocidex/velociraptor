package actions

import (
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config "www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/context"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/responder"
)

type GetClientInfo struct{}

func (self *GetClientInfo) Run(
	ctx *context.Context,
	args *crypto_proto.GrrMessage,
	output chan<- *crypto_proto.GrrMessage) {
	responder := responder.NewResponder(args, output)
	info := &actions_proto.ClientInformation{
		ClientName:    ctx.Config.Version.Name,
		ClientVersion: ctx.Config.Version.Version,
		Labels:        ctx.Config.Client.Labels,
	}
	responder.AddResponse(info)
	responder.Return()
}

type UpdateForeman struct{}

func (self *UpdateForeman) Run(
	ctx *context.Context,
	msg *crypto_proto.GrrMessage,
	output chan<- *crypto_proto.GrrMessage) {
	responder := responder.NewResponder(msg, output)
	arg, ok := responder.GetArgs().(*actions_proto.ForemanCheckin)
	if !ok {
		responder.RaiseError("Request should be of type ForemanCheckin.")
		return
	}

	if arg.LastHuntTimestamp > ctx.Config.Writeback.HuntLastTimestamp {
		ctx.Config.Writeback.HuntLastTimestamp = arg.LastHuntTimestamp
		err := config.UpdateWriteback(ctx.Config)
		if err != nil {
			responder.RaiseError(err.Error())
			return
		}
	}
	responder.Return()
}
