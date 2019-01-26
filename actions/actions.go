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
// Client actions are routines that run on the client and return a
// GrrMessage.
package actions

import (
	"context"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

type ClientAction interface {
	Run(
		config *api_proto.Config,
		ctx context.Context,
		args *crypto_proto.GrrMessage,
		output chan<- *crypto_proto.GrrMessage)
}

func GetClientActionsMap() map[string]ClientAction {
	result := make(map[string]ClientAction)
	result["GetClientInfo"] = &GetClientInfo{}
	result["VQLClientAction"] = &VQLClientAction{}
	result["GetHostname"] = &GetHostname{}
	result["GetPlatformInfo"] = &GetPlatformInfo{}
	result["UpdateForeman"] = &UpdateForeman{}
	result["UpdateEventTable"] = &UpdateEventTable{}

	return result
}
