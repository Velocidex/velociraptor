package actions

import (
	"github.com/shirou/gopsutil/host"
	assert "github.com/stretchr/testify/assert"
	"strings"
	"testing"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/context"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

func TestGetHostname(t *testing.T) {
	ctx := context.Background()
	get_hostname := GetHostname{}
	arg, err := NewRequest(&crypto_proto.GrrMessage{})
	if err != nil {
		t.Fatal(err)
	}

	responses := get_hostname.Run(&ctx, arg)
	assert.Equal(t, len(responses), 2)
	response := ExtractGrrMessagePayload(responses[0]).(*actions_proto.DataBlob)
	info, _ := host.Info()
	assert.Equal(t, info.Hostname, *response.String_)
}

func TestGetPlatformInfo(t *testing.T) {
	ctx := context.Background()
	get_platform_info := GetPlatformInfo{}
	arg, err := NewRequest(&crypto_proto.GrrMessage{})
	if err != nil {
		t.Fatal(err)
	}

	responses := get_platform_info.Run(&ctx, arg)
	assert.Equal(t, len(responses), 2)
	response := ExtractGrrMessagePayload(responses[0]).(*actions_proto.Uname)
	info, _ := host.Info()
	assert.Equal(t, strings.Title(info.OS), *response.System)
}
