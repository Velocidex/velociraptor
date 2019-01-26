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
package actions_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config "www.velocidex.com/golang/velociraptor/config"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/responder"
)

func TestClientInfo(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	args := crypto_proto.GrrMessage{}
	plugin := actions.GetClientInfo{}
	ctx := context.Background()
	responses := GetResponsesFromAction(config_obj, &plugin, ctx, &args)
	assert.Equal(t, len(responses), 2)
	assert.Equal(t, responses[1].ArgsRdfName, "GrrStatus")

	result := responder.ExtractGrrMessagePayload(
		responses[0]).(*actions_proto.ClientInformation)

	assert.Equal(t, result.ClientName, "velociraptor")
}
