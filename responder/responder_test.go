package responder_test

import (
	"github.com/stretchr/testify/assert"
	"testing"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/responder"
)

func GetResponsesFromChannel(c chan *crypto_proto.GrrMessage) []*crypto_proto.GrrMessage {
	result := []*crypto_proto.GrrMessage{}
	for {
		item, ok := <-c
		if !ok {
			return result
		}

		result = append(result, item)
	}
}

func TestResponder(t *testing.T) {
	args := &crypto_proto.GrrMessage{}
	msg := &actions_proto.ClientInformation{
		ClientName: "My name",
	}
	c := make(chan *crypto_proto.GrrMessage)
	go func() {
		defer close(c)
		responder := responder.NewResponder(args, c)
		responder.AddResponse(msg)
		responder.Return()
	}()

	response := GetResponsesFromChannel(c)

	assert.Equal(t, len(response), 2)
	assert.Equal(t, response[0].ResponseId, uint64(1))
	assert.Equal(t, response[1].ResponseId, uint64(2))

	// Last packet is the status.
	assert.Equal(t, response[1].ArgsRdfName, "GrrStatus")

	// Last packet is also marked with type status.
	assert.Equal(t, response[1].Type, crypto_proto.GrrMessage_STATUS)
}

func TestResponderError(t *testing.T) {
	args := &crypto_proto.GrrMessage{}
	error_message := "This is an error"
	c := make(chan *crypto_proto.GrrMessage)

	go func() {
		defer close(c)
		responder := responder.NewResponder(args, c)
		responder.RaiseError(error_message)
	}()

	response := GetResponsesFromChannel(c)

	assert.Equal(t, len(response), 1)
	msg := response[0]

	// Should contain a GrrStatus message.
	status := responder.ExtractGrrMessagePayload(msg).(*crypto_proto.GrrStatus)
	assert.Equal(t, status.ErrorMessage, error_message)
	assert.True(t, len(status.ErrorMessage) > 10)
}
