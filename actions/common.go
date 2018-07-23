package actions

import (
	"github.com/golang/protobuf/proto"
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
		ClientName:    *ctx.Config.Client_name,
		ClientVersion: *ctx.Config.Client_version,
		Labels:        ctx.Config.Client_labels,
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

	if arg.LastHuntTimestamp > *ctx.Config.Hunts_last_timestamp {
		ctx.Config.Hunts_last_timestamp = proto.Uint64(arg.LastHuntTimestamp)
		write_back_config := config.Config{}
		config.LoadConfig(*ctx.Config.Config_writeback, &write_back_config)
		write_back_config.Hunts_last_timestamp = ctx.Config.Hunts_last_timestamp
		err := config.WriteConfigToFile(*ctx.Config.Config_writeback,
			&write_back_config)
		if err != nil {
			responder.RaiseError(err.Error())
			return
		}
	}
	responder.Return()
}
