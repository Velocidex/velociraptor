package responder

import (
	"github.com/stretchr/testify/assert"
	"testing"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

func TestResponder(t *testing.T) {
	args := &crypto_proto.GrrMessage{}
	name := "My name"
	msg := &actions_proto.ClientInformation{
		ClientName: &name,
	}
	responder := NewResponder(args)
	responder.AddResponse(msg)
	response := responder.Return()
	assert.Equal(t, len(response), 2)
	assert.Equal(t, *response[0].ResponseId, uint64(1))
	assert.Equal(t, *response[1].ResponseId, uint64(2))

	// Last packet is the status.
	assert.Equal(t, *response[1].ArgsRdfName, "GrrStatus")

	// Last packet is also marked with type status.
	assert.Equal(t, response[1].Type, crypto_proto.GrrMessage_STATUS.Enum())
}

func TestResponderError(t *testing.T) {
	args := &crypto_proto.GrrMessage{}
	error_message := "This is an error"

	responder := NewResponder(args)
	response := responder.RaiseError(error_message)
	assert.Equal(t, len(response), 1)
	msg := response[0]

	// Should contain a GrrStatus message.
	status := ExtractGrrMessagePayload(msg).(*crypto_proto.GrrStatus)
	assert.Equal(t, *status.ErrorMessage, error_message)
	assert.True(t, len(*status.ErrorMessage) > 10)
}
