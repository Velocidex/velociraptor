package actions

import (
	"context"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config "www.velocidex.com/golang/velociraptor/config"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/responder"
)

type GetClientInfo struct{}

func (self *GetClientInfo) Run(
	config *api_proto.Config,
	ctx context.Context,
	args *crypto_proto.GrrMessage,
	output chan<- *crypto_proto.GrrMessage) {
	responder := responder.NewResponder(args, output)
	info := &actions_proto.ClientInformation{
		ClientName:    config.Version.Name,
		ClientVersion: config.Version.Version,
		Labels:        config.Client.Labels,
	}
	responder.AddResponse(info)
	responder.Return()
}

type UpdateForeman struct{}

func (self *UpdateForeman) Run(
	config_obj *api_proto.Config,
	ctx context.Context,
	msg *crypto_proto.GrrMessage,
	output chan<- *crypto_proto.GrrMessage) {
	responder := responder.NewResponder(msg, output)
	arg, ok := responder.GetArgs().(*actions_proto.ForemanCheckin)
	if !ok {
		responder.RaiseError("Request should be of type ForemanCheckin.")
		return
	}

	if arg.LastHuntTimestamp > config_obj.Writeback.HuntLastTimestamp {
		config_obj.Writeback.HuntLastTimestamp = arg.LastHuntTimestamp
		err := config.UpdateWriteback(config_obj)
		if err != nil {
			responder.RaiseError(err.Error())
			return
		}
	}
	responder.Return()
}
