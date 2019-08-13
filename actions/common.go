/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package actions

import (
	"context"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config "www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/responder"
)

type GetClientInfo struct{}

func (self *GetClientInfo) Run(
	config *config_proto.Config,
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
	config_obj *config_proto.Config,
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
