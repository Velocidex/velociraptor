package actions_test

import (
	"github.com/shirou/gopsutil/host"
	assert "github.com/stretchr/testify/assert"
	"strings"
	"testing"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/context"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/responder"
)

func GetResponsesFromAction(
	action actions.ClientAction,
	ctx *context.Context,
	args *crypto_proto.GrrMessage) []*crypto_proto.GrrMessage {
	c := make(chan *crypto_proto.GrrMessage)
	result := []*crypto_proto.GrrMessage{}

	go func() {
		defer close(c)
		action.Run(ctx, args, c)
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
	ctx := context.Background()
	get_hostname := actions.GetHostname{}
	arg, err := responder.NewRequest(&crypto_proto.GrrMessage{})
	if err != nil {
		t.Fatal(err)
	}

	responses := GetResponsesFromAction(&get_hostname, &ctx, arg)
	assert.Equal(t, len(responses), 2)
	response := responder.ExtractGrrMessagePayload(
		responses[0]).(*actions_proto.DataBlob)
	info, _ := host.Info()
	assert.Equal(t, info.Hostname, *response.String_)
}

func TestGetPlatformInfo(t *testing.T) {
	ctx := context.Background()
	get_platform_info := actions.GetPlatformInfo{}
	arg, err := responder.NewRequest(&crypto_proto.GrrMessage{})
	if err != nil {
		t.Fatal(err)
	}

	responses := GetResponsesFromAction(&get_platform_info, &ctx, arg)
	assert.Equal(t, len(responses), 2)
	response := responder.ExtractGrrMessagePayload(
		responses[0]).(*actions_proto.Uname)
	info, _ := host.Info()
	assert.Equal(t, strings.Title(info.OS), *response.System)
}
