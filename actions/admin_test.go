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
	"strings"
	"testing"

	"github.com/shirou/gopsutil/host"
	assert "github.com/stretchr/testify/assert"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/responder"
)

func GetResponsesFromAction(
	config_obj *config_proto.Config,
	action actions.ClientAction,
	ctx context.Context,
	args *crypto_proto.GrrMessage) []*crypto_proto.GrrMessage {
	c := make(chan *crypto_proto.GrrMessage)
	result := []*crypto_proto.GrrMessage{}

	go func() {
		defer close(c)
		action.Run(config_obj, ctx, args, c)
	}()

	for {
		item, ok := <-c
		if !ok {
			return result
		}

		result = append(result, item)
	}
}

func TestGetHostname(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	ctx := context.Background()
	get_hostname := actions.GetHostname{}
	arg, err := responder.NewRequest(&crypto_proto.GrrMessage{})
	if err != nil {
		t.Fatal(err)
	}

	responses := GetResponsesFromAction(config_obj, &get_hostname, ctx, arg)
	assert.Equal(t, len(responses), 2)
	response := responder.ExtractGrrMessagePayload(
		responses[0]).(*actions_proto.DataBlob)
	info, _ := host.Info()
	assert.Equal(t, info.Hostname, response.String_)
}

func TestGetPlatformInfo(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	ctx := context.Background()
	get_platform_info := actions.GetPlatformInfo{}
	arg, err := responder.NewRequest(&crypto_proto.GrrMessage{})
	if err != nil {
		t.Fatal(err)
	}

	responses := GetResponsesFromAction(config_obj, &get_platform_info, ctx, arg)
	assert.Equal(t, len(responses), 2)
	response := responder.ExtractGrrMessagePayload(
		responses[0]).(*actions_proto.Uname)
	info, _ := host.Info()
	assert.Equal(t, strings.Title(info.OS), response.System)
}
