package actions

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"www.velocidex.com/golang/velociraptor/context"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)


func TestClientInfo(t *testing.T) {
	args := crypto_proto.GrrMessage{}
	plugin := GetClientInfo{}
	ctx := context.Background()
	responses := plugin.Run(&ctx, &args)
	assert.Equal(t, len(responses), 2)
	assert.Equal(t, *responses[1].ArgsRdfName, "GrrStatus")

	result := ExtractGrrMessagePayload(responses[0]).(
		*actions_proto.ClientInformation)

	assert.Equal(t, *result.ClientName, "velociraptor")
}
