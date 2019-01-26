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
