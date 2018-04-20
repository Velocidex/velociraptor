package actions

import (
	"reflect"
	"strings"
	"runtime/debug"
	"github.com/golang/protobuf/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)


type Responder struct {
	request *crypto_proto.GrrMessage
	responses []*crypto_proto.GrrMessage
}

// NewResponder returns a new Responder.
func NewResponder(request *crypto_proto.GrrMessage) *Responder {
	result	:= &Responder{request: request}
	return result
}

func (self *Responder) AddResponse(message proto.Message) error {
	components := strings.Split(proto.MessageName(message), ".")
	rdf_name := components[len(components)-1]
	response_id := uint64(len(self.responses)) + 1
	response := &crypto_proto.GrrMessage{
		SessionId: self.request.SessionId,
		RequestId: self.request.RequestId,
		ResponseId: &response_id,
		ArgsRdfName: &rdf_name,
	}

	serialized_args, err := proto.Marshal(message)
	if err != nil {
		return err
	}
	response.Args = serialized_args

	if rdf_name == "GrrStatus" {
		response.Type = crypto_proto.GrrMessage_STATUS.Enum()
	}

	self.responses	= append(self.responses, response)

	return nil
}

func (self *Responder) RaiseError(message string) []*crypto_proto.GrrMessage {
	backtrace := string(debug.Stack())
	status := &crypto_proto.GrrStatus{
		Backtrace: &backtrace,
		ErrorMessage: &message,
		Status: crypto_proto.GrrStatus_GENERIC_ERROR.Enum(),
	}
	self.AddResponse(status)

	return self.responses
}

func (self *Responder) Return() []*crypto_proto.GrrMessage {
	status := &crypto_proto.GrrStatus{
		Status: crypto_proto.GrrStatus_OK.Enum(),
	}
	self.AddResponse(status)
	return self.responses
}


// Unpack the GrrMessage payload. The return value should be type asserted.
func ExtractGrrMessagePayload(message *crypto_proto.GrrMessage) proto.Message {
	message_type := proto.MessageType("proto." + *message.ArgsRdfName)
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
