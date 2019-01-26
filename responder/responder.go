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
package responder

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"reflect"
	"runtime/debug"
	"strings"
	"time"
	constants "www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

type Responder struct {
	request *crypto_proto.GrrMessage
	next_id uint64
	output  chan<- *crypto_proto.GrrMessage
}

// NewResponder returns a new Responder.
func NewResponder(
	request *crypto_proto.GrrMessage,
	output chan<- *crypto_proto.GrrMessage) *Responder {
	result := &Responder{
		request: request,
		next_id: 0,
		output:  output,
	}
	return result
}

func (self *Responder) AddResponse(message proto.Message) error {
	return self.AddResponseToRequest(self.request.RequestId, message)
}

func (self *Responder) AddResponseToRequest(
	request_id uint64, message proto.Message) error {
	components := strings.Split(proto.MessageName(message), ".")
	rdf_name := components[len(components)-1]
	self.next_id = self.next_id + 1
	response := &crypto_proto.GrrMessage{
		SessionId:   self.request.SessionId,
		RequestId:   request_id,
		ResponseId:  self.next_id,
		ArgsRdfName: rdf_name,
		ClientType:  crypto_proto.GrrMessage_VELOCIRAPTOR,
	}
	response.TaskId = self.request.TaskId

	serialized_args, err := proto.Marshal(message)
	if err != nil {
		return err
	}
	response.Args = serialized_args

	if rdf_name == "GrrStatus" {
		response.Type = crypto_proto.GrrMessage_STATUS
	}

	self.output <- response

	return nil
}

func (self *Responder) RaiseError(message string) {
	backtrace := string(debug.Stack())
	status := &crypto_proto.GrrStatus{
		Backtrace:    backtrace,
		ErrorMessage: message,
		Status:       crypto_proto.GrrStatus_GENERIC_ERROR,
	}
	self.AddResponse(status)
}

func (self *Responder) Return() {
	status := &crypto_proto.GrrStatus{
		Status: crypto_proto.GrrStatus_OK,
	}
	self.AddResponse(status)
}

// Send a log message to the server.
func (self *Responder) Log(format string, v ...interface{}) error {
	msg := &crypto_proto.LogMessage{
		Message:   fmt.Sprintf(format, v...),
		Timestamp: uint64(time.Now().UTC().UnixNano() / 1000),
	}
	return self.AddResponseToRequest(constants.LOG_SINK, msg)
}

func (self *Responder) SendResponseToWellKnownFlow(
	flow_name string, message proto.Message) error {
	components := strings.Split(proto.MessageName(message), ".")
	rdf_name := components[len(components)-1]
	response := &crypto_proto.GrrMessage{
		SessionId:   flow_name,
		ResponseId:  1,
		ArgsRdfName: rdf_name,
		ClientType:  crypto_proto.GrrMessage_VELOCIRAPTOR,
	}

	response.TaskId = self.request.TaskId
	serialized_args, err := proto.Marshal(message)
	if err != nil {
		return err
	}
	response.Args = serialized_args
	self.output <- response
	return nil
}

func (self *Responder) GetArgs() proto.Message {
	return ExtractGrrMessagePayload(self.request)
}

func (self *Responder) SessionId() string {
	return self.request.SessionId
}

// Unpack the GrrMessage payload. The return value should be type asserted.
func ExtractGrrMessagePayload(message *crypto_proto.GrrMessage) proto.Message {
	message_type := proto.MessageType("proto." + message.ArgsRdfName)
	if message_type != nil {
		new_message := reflect.New(message_type.Elem()).Interface().(proto.Message)
		err := proto.Unmarshal(message.Args, new_message)
		if err != nil {
			return nil
		}
		return new_message
	}
	return nil
}

func NewRequest(message proto.Message) (*crypto_proto.GrrMessage, error) {
	components := strings.Split(proto.MessageName(message), ".")
	rdf_name := components[len(components)-1]
	response := &crypto_proto.GrrMessage{
		SessionId:   "XYZ",
		RequestId:   1,
		ArgsRdfName: rdf_name,
		ClientType:  crypto_proto.GrrMessage_VELOCIRAPTOR,
	}

	serialized_args, err := proto.Marshal(message)
	if err != nil {
		return nil, err
	}
	response.Args = serialized_args

	return response, nil
}
