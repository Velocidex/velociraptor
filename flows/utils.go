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
package flows

import (
	"github.com/golang/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	datastore "www.velocidex.com/golang/velociraptor/datastore"
)

// ProduceBackwardCompatibleGrrMessage is used for messages going from
// the server to the client. In order to support old clients we
// duplicate the data in the extra fields.
func ProduceBackwardCompatibleGrrMessage(req *crypto_proto.GrrMessage) *crypto_proto.GrrMessage {
	var payload proto.Message

	// Only bother for server -> client messages since this we
	// only support older clients on newer servers.
	if req.UpdateEventTable != nil {
		payload = req.UpdateEventTable
		req.Name = "UpdateEventTable"
		req.ArgsRdfName = "VQLEventTable"
	}

	if req.VQLClientAction != nil {
		payload = req.VQLClientAction
		req.Name = "VQLClientAction"
		req.ArgsRdfName = "VQLCollectorArgs"
	}

	if req.UpdateForeman != nil {
		payload = req.UpdateForeman
		req.Name = "UpdateForeman"
		req.ArgsRdfName = "ForemanCheckin"
	}

	if payload != nil {
		serialized, err := proto.Marshal(payload)
		if err == nil {
			req.Args = serialized
		}
	}

	return req
}

func QueueMessageForClient(
	config_obj *config_proto.Config,
	client_id string,
	req *crypto_proto.GrrMessage) error {

	req = ProduceBackwardCompatibleGrrMessage(req)

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	return db.QueueMessageForClient(config_obj, client_id, req)
}
