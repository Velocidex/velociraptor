package actions_test

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/context"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/responder"
)

func TestClientInfo(t *testing.T) {
	args := crypto_proto.GrrMessage{}
	plugin := actions.GetClientInfo{}
	ctx := context.Background()
	responses := GetResponsesFromAction(&plugin, &ctx, &args)
	assert.Equal(t, len(responses), 2)
	assert.Equal(t, *responses[1].ArgsRdfName, "GrrStatus")

	result := responder.ExtractGrrMessagePayload(
		responses[0]).(*actions_proto.ClientInformation)

	assert.Equal(t, *result.ClientName, "velociraptor")
}
