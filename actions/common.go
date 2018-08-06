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
		ClientName:    ctx.Config.Client.Name,
		ClientVersion: ctx.Config.Client.Version,
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

	if arg.LastHuntTimestamp > ctx.Config.Client.HuntLastTimestamp {
		ctx.Config.Client.HuntLastTimestamp = arg.LastHuntTimestamp
		write_back_config := config.NewClientConfig()
		config.LoadConfig(ctx.Config.Writeback, write_back_config)
		write_back_config.Client.HuntLastTimestamp = ctx.Config.Client.HuntLastTimestamp
		err := config.WriteConfigToFile(ctx.Config.Writeback,
			write_back_config)
		if err != nil {
			responder.RaiseError(err.Error())
			return
		}
	}
	responder.Return()
}
